package cmd

import (
	"context"
	"fmt"

	"github.com/openhue/openhue-go"
	"github.com/samvdb/loxone-philips-hue/client"
	"github.com/samvdb/loxone-philips-hue/udp"
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
	flagLoxoneIP         string
	flagLoxoneUdpPort    int
	flagPhilipsHueIP     string
	flagPhilipsHueApiKey string
	debug                bool
)

var rootCmd = &cobra.Command{
	Use: "",
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

	// Handle SIGINT/SIGTERM for graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sigs
		slog.Info("signal received, shutting down", "signal", s.String())
		cancel()
	}()

	// UDP server (listen on all interfaces, same port as Loxone or pick your own)
	// Commonly Loxone will send to us on some port; expose it with a flag if you like.
	serverAddr := &net.UDPAddr{IP: net.IPv4zero, Port: flagLoxoneUdpPort}
	udpServer, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		return fmt.Errorf("listen UDP: %w", err)
	}
	defer udpServer.Close()

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
	rootCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logging")
	rootCmd.Flags().StringVar(&flagLoxoneIP, "loxone-ip", "", "Loxone IP")
	rootCmd.Flags().IntVar(&flagLoxoneUdpPort, "loxone-udp-port", 1234, "Loxone's UDP server port")
	rootCmd.Flags().StringVar(&flagPhilipsHueIP, "philips-hue-ip", "", "Philips Hue IP")
	rootCmd.Flags().StringVar(&flagPhilipsHueApiKey, "philips-hue-apikey", "", "Philips Hue API Key")
	// Read from environment if flag not set
	if v := os.Getenv("LOXONE_IP"); v != "" {
		flagLoxoneIP = v
	}
	if v := os.Getenv("DEBUG"); v != "" {
		if p, err := strconv.ParseBool(v); err == nil {
			debug = p
		}
	}
	if v := os.Getenv("PHILIPS_HUE_IP"); v != "" {
		flagPhilipsHueIP = v
	}
	if v := os.Getenv("PHILIPS_HUE_APIKEY"); v != "" {
		flagPhilipsHueApiKey = v
	}
	if v := os.Getenv("LOXONE_UDP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			flagLoxoneUdpPort = p
		}
	}
	_ = rootCmd.MarkFlagRequired("loxone-ip")
	_ = rootCmd.MarkFlagRequired("loxone-udp-port")
	_ = rootCmd.MarkFlagRequired("philips-hue-ip")
	_ = rootCmd.MarkFlagRequired("philips-hue-apikey")
}
