package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/openhue/openhue-go"
	"github.com/samvdb/loxone-philips-hue/client"
	"github.com/samvdb/loxone-philips-hue/udp"

	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"

	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var (
	cfgFile              string
	flagLoxoneIP         string
	flagLoxoneUdpPort    int
	flagPhilipsHueIP     string
	flagPhilipsHueApiKey string
	debug                bool
)

var rootCmd = &cobra.Command{
	Use: "",

	PreRunE: func(cmd *cobra.Command, arg []string) error {
		return viper.BindPFlags(cmd.Flags())
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// ---- validation
		if flagLoxoneIP == "" {
			return fmt.Errorf("--loxone-ip is required")
		}
		if flagLoxoneUdpPort <= 0 || flagLoxoneUdpPort > 65535 {
			return fmt.Errorf("--loxone-udp-port must be a valid UDP port")
		}
		if flagPhilipsHueIP == "" {
			return fmt.Errorf("--philips-hue-ip is required")
		}
		if flagPhilipsHueApiKey == "" {
			return fmt.Errorf("--philips-hue-apikey is required")
		}

		level := slog.LevelInfo
		if debug {
			level = slog.LevelDebug
		}
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
		slog.SetDefault(logger)

		return Run(cmd)
	},
}

func Run(cmd *cobra.Command) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// UDP server (listen on all interfaces, same port as Loxone or pick your own)
	// Commonly Loxone will send to us on some port; expose it with a flag if you like.
	//serverAddr := &net.UDPAddr{IP: net.IPv4zero, Port: flagLoxoneUdpPort}
	//udpServer, err := net.ListenUDP("udp", serverAddr)
	//if err != nil {
	//	return fmt.Errorf("listen UDP: %w", err)
	//}
	//defer udpServer.Close()

	clientLogger := slog.With("module", "client", "loxone_ip", flagLoxoneIP, "loxone_udp_port", flagLoxoneUdpPort)
	udpClient, err := udp.NewClient(ctx, udp.ClientConfig{
		Remote:          net.JoinHostPort(flagLoxoneIP, strconv.Itoa(flagLoxoneUdpPort)),
		WriteTimeout:    1 * time.Second,
		QueueSize:       1024,
		BaseBackoff:     250 * time.Millisecond,
		MaxBackoff:      8 * time.Second,
		ResolveInterval: 0, // re-resolve every reconnect; or set e.g. 1m
		Logger:          clientLogger,
	})
	if err != nil {
		return err
	}
	hueHome, err := openhue.NewHome(flagPhilipsHueIP, flagPhilipsHueApiKey)
	if err != nil {
		return err
	}
	defer udpClient.Close()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {

		poller := client.NewPoller(ctx, hueHome, udpClient)
		for {
			err := poller.Run()
			if err != nil {
				slog.Error("poller run failed", "error", err.Error())
			}
			select {
			case <-time.After(10 * time.Second):
			case <-ctx.Done():
				return nil
			}
		}
	})

	return g.Wait()
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Path to config file (json|yaml|toml)")
	rootCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logging")
	rootCmd.Flags().StringVar(&flagLoxoneIP, "loxone-ip", "", "Loxone IP")
	rootCmd.Flags().IntVar(&flagLoxoneUdpPort, "loxone-udp-port", 1234, "Loxone's UDP server port")
	rootCmd.Flags().StringVar(&flagPhilipsHueIP, "philips-hue-ip", "", "Philips Hue IP")
	rootCmd.Flags().StringVar(&flagPhilipsHueApiKey, "philips-hue-apikey", "", "Philips Hue API Key")

	// Bind every flag to Viper keys
	_ = viper.BindPFlag("debug", rootCmd.Flags().Lookup("debug"))
	_ = viper.BindPFlag("loxone_ip", rootCmd.Flags().Lookup("loxone-ip"))
	_ = viper.BindPFlag("loxone_udp_port", rootCmd.Flags().Lookup("loxone-udp-port"))
	_ = viper.BindPFlag("philips_hue_ip", rootCmd.Flags().Lookup("philips-hue-ip"))
	_ = viper.BindPFlag("philips_hue_apikey", rootCmd.Flags().Lookup("philips-hue-apikey"))

	// Env: MYAPP_LOXONE_IP, MYAPP_DEBUG, etc.
	viper.SetEnvPrefix("")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	viper.AddConfigPath(".")

	// Load config before RunE
	cobra.OnInitialize(initConfig)
	//
	//_ = rootCmd.MarkFlagRequired("loxone-ip")
	//_ = rootCmd.MarkFlagRequired("loxone-udp-port")
	//_ = rootCmd.MarkFlagRequired("philips-hue-ip")
	//_ = rootCmd.MarkFlagRequired("philips-hue-apikey")

}
func initConfig() {
	// If --config has been provided, use that file
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// Default: look for config.json in CWD
		viper.SetConfigName("config")
		viper.SetConfigType("json")
		viper.AddConfigPath(".")
	}

	// Only load config if it exists
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
