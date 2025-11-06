package cmd

import (
	"context"
	"fmt"
	"strings"

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
	RunE: func(cmd *cobra.Command, args []string) error {

		level := slog.LevelInfo
		if debug {
			level = slog.LevelDebug
		}
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
		slog.SetDefault(logger)
		return Run(cmd)
	},
}

func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {

	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Path to config file (json|yaml|toml)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging")
	rootCmd.PersistentFlags().StringVar(&flagLoxoneIP, "loxone-ip", "", "Loxone IP")
	rootCmd.PersistentFlags().IntVar(&flagLoxoneUdpPort, "loxone-udp-port", 1234, "Loxone's UDP server port")
	rootCmd.PersistentFlags().StringVar(&flagPhilipsHueIP, "philips-hue-ip", "", "Philips Hue IP")
	rootCmd.PersistentFlags().StringVar(&flagPhilipsHueApiKey, "philips-hue-apikey", "", "Philips Hue API Key")

	// Bind flags → Viper config keys
	_ = viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
	_ = viper.BindPFlag("loxone_ip", rootCmd.PersistentFlags().Lookup("loxone-ip"))
	_ = viper.BindPFlag("loxone_udp_port", rootCmd.PersistentFlags().Lookup("loxone-udp-port"))
	_ = viper.BindPFlag("philips_hue_ip", rootCmd.PersistentFlags().Lookup("philips-hue-ip"))
	_ = viper.BindPFlag("philips_hue_apikey", rootCmd.PersistentFlags().Lookup("philips-hue-apikey"))

	// Env: MYAPP_LOXONE_IP, MYAPP_DEBUG, etc.
	viper.SetEnvPrefix("")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	viper.AddConfigPath(".")
}
func initConfig() {
	// If --config has been provided, use that file
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// Default: look for .config.json in CWD
		viper.SetConfigName(".config")
		viper.SetConfigType("json")
		viper.AddConfigPath(".")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		slog.Info(fmt.Sprintf("Using config file: %s", viper.ConfigFileUsed()))

		slog.Info("--- Configuration read from Viper ---")

		for key, value := range viper.AllSettings() {
			slog.Info("config",
				"key", key,
				"value", fmt.Sprintf("%v", value),
			)
		}

		slog.Info("--- End of configuration ---")

	}
	debug = viper.GetBool("debug")
	flagLoxoneIP = viper.GetString("loxone_ip")
	flagLoxoneUdpPort = viper.GetInt("loxone_udp_port")
	flagPhilipsHueIP = viper.GetString("philips_hue_ip")
	flagPhilipsHueApiKey = viper.GetString("philips_hue_apikey")
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
	// slog.Debug("connect to home bridge", "ip", flagPhilipsHueIP, "apikey", flagPhilipsHueApiKey)
	// hueHome, err := openhue.NewHome(flagPhilipsHueIP, flagPhilipsHueApiKey)
	// if err != nil {
	// 	return err
	// }
	defer udpClient.Close()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {

		poller := client.NewStreamer(ctx, flagPhilipsHueIP, flagPhilipsHueApiKey, udpClient)
		err := poller.Run(ctx)
		if err != nil {
			slog.Error("poller run failed", "error", err.Error())
		}

		return err

	})

	return g.Wait()
}
