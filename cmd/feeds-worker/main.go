package main

import (
	"fmt"
	"os"

	"github.com/Tarick/naca-items/pkg/itempublisher"
	"github.com/Tarick/naca-rss-feeds/internal/application/worker"
	"github.com/Tarick/naca-rss-feeds/internal/logger/zaplogger"
	"github.com/Tarick/naca-rss-feeds/internal/messaging/nsqclient/consumer"
	"github.com/Tarick/naca-rss-feeds/internal/messaging/nsqclient/producer"
	"github.com/Tarick/naca-rss-feeds/internal/processor"
	"github.com/Tarick/naca-rss-feeds/internal/repository/postgresql"
	"github.com/Tarick/naca-rss-feeds/internal/tracing"
	"github.com/Tarick/naca-rss-feeds/internal/version"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func main() {
	var (
		cfgFile string
	)
	// rootCmd represents the base command when called without any subcommands
	rootCmd := &cobra.Command{
		Use:   "rss-feeds-worker",
		Short: "RSS feeds worker to fetch and parse feeds",
		Long:  `Command line worker for RSS/Atom feeds retrieval and news item producing`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return startWorker(cfgFile)
		},
	}
	// Version command, attached to root
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number of application",
		Long:  `Software version`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("NACA RSS Feeds worker version:", version.Version, "build on:", version.BuildTime)
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// We read config file and use dependency injection to create worker
func startWorker(cfgFile string) error {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")      // optionally look for config in the working directory
		viper.SetConfigName("config") // name of config file (without extension)
		// viper.SetConfigType("yaml") // REQUIRED if the config file does not have the extension in the name
	}
	// If the config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("FATAL: error in config file %s, %v", viper.ConfigFileUsed(), err)
	}
	fmt.Println("Using config file:", viper.ConfigFileUsed())
	// Init logging
	logCfg := &zaplogger.Config{}
	if err := viper.UnmarshalKey("logging", logCfg); err != nil {
		return fmt.Errorf("FATAL: Failure reading 'logging' configuration, %v", err)
	}
	logger := zaplogger.New(logCfg).Sugar()
	defer logger.Sync()

	// Init tracing
	tracingCfg := tracing.Config{}
	if err := viper.UnmarshalKey("tracing", &tracingCfg); err != nil {
		return fmt.Errorf("FATAL: Failure reading 'tracing' configuration, %v", err)
	}
	tracer, tracerCloser, err := tracing.New(tracingCfg, tracing.NewZapLogger(logger))
	defer tracerCloser.Close()
	if err != nil {
		return fmt.Errorf("FATAL: Cannot init tracing, %v", err)
	}

	// Create db configuration
	databaseViperConfig := viper.Sub("database")
	dbCfg := &postgresql.Config{}
	if err := databaseViperConfig.UnmarshalExact(dbCfg); err != nil {
		return fmt.Errorf("FATAL: failure reading 'database' configuration: %v", err)
	}
	// Open db
	db, err := postgresql.New(dbCfg, postgresql.NewZapLogger(logger.Desugar()), tracer)
	if err != nil {
		return fmt.Errorf("FATAL: failure creating database connection, %v", err)
	}

	// Create NSQ producer
	publishViperConfig := viper.Sub("publish")
	publishCfg := &producer.MessageProducerConfig{}
	if err := publishViperConfig.UnmarshalExact(&publishCfg); err != nil {
		return fmt.Errorf("FATAL: failure reading NSQ 'publish' configuration, %v", err)
	}
	messageProducer, err := producer.New(publishCfg)
	if err != nil {
		return fmt.Errorf("FATAL: failure initialising NSQ producer, %v", err)
	}
	rssFeedsUpdateProducer := processor.NewFeedsUpdateProducer(messageProducer, tracer)

	consumeViperConfig := viper.Sub("consume")
	consumeCfg := &consumer.MessageConsumerConfig{}
	if err := consumeViperConfig.UnmarshalExact(&consumeCfg); err != nil {
		return fmt.Errorf("FATAL: failure reading 'consume' configuration, %v", err)
	}
	itemPublisherClientViperConfig := viper.Sub("itemPublish")
	// FIXME: rather unclear initialization of config
	itemPublisherClientCfg := struct {
		Host  string `mapstructure:"host"`
		Topic string `mapstructure:"topic"`
	}{}
	if err := itemPublisherClientViperConfig.UnmarshalExact(&itemPublisherClientCfg); err != nil {
		return fmt.Errorf("FATAL: failure reading 'itemPublish' configuration, %v", err)
	}
	itemPublisherClient, err := itempublisher.New(itemPublisherClientCfg.Host, itemPublisherClientCfg.Topic)
	if err != nil {
		return fmt.Errorf("FATAL: failure creating itemPublisher client, %v", err)
	}
	// Construct consumer with message handler
	rssFeedsProcessor := processor.NewRSSFeedsProcessor(db, rssFeedsUpdateProducer, itemPublisherClient, logger, tracer)
	consumer, err := consumer.New(consumeCfg, rssFeedsProcessor, logger)
	if err != nil {
		return fmt.Errorf("FATAL: consumer creation failed, %v", err)
	}
	wrkr := worker.New(consumer, logger)
	return wrkr.Start()
}
