package main

//go:generate swagger generate spec --scan-models -o ../../internal/docs/swagger.json

import (
	"fmt"
	"os"

	_ "github.com/Tarick/naca-rss-feeds/internal/docs"
	"github.com/Tarick/naca-rss-feeds/internal/logger/zaplogger"

	"github.com/Tarick/naca-rss-feeds/internal/application/server"
	"github.com/Tarick/naca-rss-feeds/internal/messaging/nsqclient/producer"
	"github.com/Tarick/naca-rss-feeds/internal/processor"
	"github.com/Tarick/naca-rss-feeds/internal/repository/postgresql"
	"github.com/Tarick/naca-rss-feeds/internal/tracing"
	"github.com/Tarick/naca-rss-feeds/internal/version"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func main() {
	var cfgFile string

	// rootCmd represents the base command when called without any subcommands
	rootCmd := &cobra.Command{
		Use:   "rss-feeds-api",
		Short: "RSS Feeds API",
		Long:  `RSS Feeds API`,
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := configure(cfgFile)
			if err != nil {
				return err
			}
			return server.StartAndServe()
		},
	}
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number of application",
		Long:  `Software version`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("NACA RSS Feeds API version:", version.Version, ",build on:", version.BuildTime)
		},
	}
	rootCmd.AddCommand(versionCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// configure parses configuration file, uses depencency injection to create and return server
func configure(cfgFile string) (*server.Server, error) {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")      // optionally look for config in the working directory
		viper.SetConfigName("config") // name of config file (without extension)
	}
	// If the config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("FATAL: error in config file %s. %v", viper.ConfigFileUsed(), err)
	}

	fmt.Println("Using config file:", viper.ConfigFileUsed())
	// Init logging
	logCfg := &zaplogger.Config{}
	if err := viper.UnmarshalKey("logging", logCfg); err != nil {
		return nil, fmt.Errorf("Failure reading 'logging' configuration, %v", err)
	}
	logger := zaplogger.New(logCfg).Sugar()
	defer logger.Sync()

	// Init tracing
	tracingCfg := tracing.Config{}
	if err := viper.UnmarshalKey("tracing", &tracingCfg); err != nil {
		return nil, fmt.Errorf("Failure reading 'tracing' configuration, %v", err)
	}
	tracer, tracerCloser, err := tracing.New(tracingCfg, tracing.NewZapLogger(logger))
	defer tracerCloser.Close()
	if err != nil {
		return nil, fmt.Errorf("FATAL: Cannot init tracing, %v", err)
	}

	// Create db configuration
	databaseViperConfig := viper.Sub("database")
	dbCfg := &postgresql.Config{}
	if err := databaseViperConfig.UnmarshalExact(dbCfg); err != nil {
		return nil, fmt.Errorf("FATAL: failure reading 'database' configuration, %v", err)
	}
	// Open db
	db, err := postgresql.New(dbCfg, postgresql.NewZapLogger(logger.Desugar()), tracer)
	if err != nil {
		return nil, fmt.Errorf("FATAL: failure creating database connection, %v", err)
	}

	// Create NSQ producer
	publishViperConfig := viper.Sub("publish")
	publishCfg := &producer.MessageProducerConfig{}
	if err := publishViperConfig.UnmarshalExact(&publishCfg); err != nil {
		return nil, fmt.Errorf("FATAL: failure reading NSQ 'publish' configuration, %v", err)
	}
	messageProducer, err := producer.New(publishCfg)
	if err != nil {
		return nil, fmt.Errorf("FATAL: failure initialising NSQ producer, %v", err)
	}
	rssFeedsUpdateProducer := processor.NewFeedsUpdateProducer(messageProducer, tracer)
	// Create web server
	serverCfg := server.Config{}
	serverViperConfig := viper.Sub("server")
	if err := serverViperConfig.UnmarshalExact(&serverCfg); err != nil {
		return nil, fmt.Errorf("FATAL: failure reading 'server' configuration, %v", err)
	}
	handler := server.NewHandler(logger, tracer, db, rssFeedsUpdateProducer)
	return server.New(serverCfg, logger, handler), nil
}
