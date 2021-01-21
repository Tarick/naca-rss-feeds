package main

import (
	"fmt"
	"os"

	"github.com/Tarick/naca-items/pkg/itempublisher"
	"github.com/Tarick/naca-rss-feeds/internal/application/worker"
	"github.com/Tarick/naca-rss-feeds/internal/logger/zaplogger"
	"github.com/Tarick/naca-rss-feeds/internal/messaging"
	"github.com/Tarick/naca-rss-feeds/internal/messaging/nsqclient/consumer"
	"github.com/Tarick/naca-rss-feeds/internal/messaging/nsqclient/producer"
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
		Run: func(cmd *cobra.Command, args []string) {
			if cfgFile != "" {
				// Use config file from the flag.
				viper.SetConfigFile(cfgFile)
			} else {
				viper.AddConfigPath(".")      // optionally look for config in the working directory
				viper.SetConfigName("config") // name of config file (without extension)
				// viper.SetConfigType("yaml") // REQUIRED if the config file does not have the extension in the name
			}
			// If the config file is found, read it in.
			if err := viper.ReadInConfig(); err != nil {
				fmt.Printf("FATAL: error in config file %s. %s", viper.ConfigFileUsed(), err)
				os.Exit(1)
			}
			fmt.Println("Using config file:", viper.ConfigFileUsed())
			// Init logging
			logCfg := &zaplogger.Config{}
			if err := viper.UnmarshalKey("logging", logCfg); err != nil {
				fmt.Println("Failure reading 'logging' configuration:", err)
				os.Exit(1)
			}
			logger := zaplogger.New(logCfg).Sugar()
			defer logger.Sync()

			// Init tracing
			tracingCfg := tracing.Config{}
			if err := viper.UnmarshalKey("tracing", &tracingCfg); err != nil {
				fmt.Println("Failure reading 'tracing' configuration:", err)
				os.Exit(1)
			}
			tracer, tracerCloser := tracing.New(tracingCfg)
			defer tracerCloser.Close()

			// Create db configuration
			databaseViperConfig := viper.Sub("database")
			dbCfg := &postgresql.Config{}
			if err := databaseViperConfig.UnmarshalExact(dbCfg); err != nil {
				fmt.Println("FATAL: failure reading 'database' configuration: ", err)
				os.Exit(1)
			}
			// Open db
			db, err := postgresql.New(dbCfg, postgresql.NewZapLogger(logger.Desugar()), tracer)
			if err != nil {
				fmt.Println("FATAL: failure creating database connection, ", err)
				os.Exit(1)
			}

			// Create NSQ producer
			publishViperConfig := viper.Sub("publish")
			publishCfg := &producer.MessageProducerConfig{}
			if err := publishViperConfig.UnmarshalExact(&publishCfg); err != nil {
				fmt.Println("FATAL: failure reading NSQ 'publish' configuration, ", err)
				os.Exit(1)
			}
			messageProducer, err := producer.New(publishCfg)
			if err != nil {
				fmt.Println("FATAL: failure initialising NSQ producer, ", err)
				os.Exit(1)
			}
			rssFeedsUpdateProducer := messaging.NewFeedsUpdateProducer(messageProducer, tracer)

			consumeViperConfig := viper.Sub("consume")
			consumeCfg := &consumer.MessageConsumerConfig{}
			if err := consumeViperConfig.UnmarshalExact(&consumeCfg); err != nil {
				fmt.Println("FATAL: failure reading 'consume' configuration, ", err)
				os.Exit(1)
			}
			itemPublisherClientViperConfig := viper.Sub("itemPublish")
			itemPublisherClientCfg := struct {
				Host  string `mapstructure:"host"`
				Topic string `mapstructure:"topic"`
			}{}
			if err := itemPublisherClientViperConfig.UnmarshalExact(&itemPublisherClientCfg); err != nil {
				fmt.Println("FATAL: failure reading 'itemPublish' configuration, ", err)
				os.Exit(1)
			}
			itemPublisherClient, err := itempublisher.New(itemPublisherClientCfg.Host, itemPublisherClientCfg.Topic)
			if err != nil {
				fmt.Println("FATAL: failure creating itemPublisher client, ", err)
				os.Exit(1)
			}
			// Construct consumer with message handler
			rssFeedsProcessor := messaging.NewRSSFeedsProcessor(db, rssFeedsUpdateProducer, itemPublisherClient, logger, tracer)
			consumer, err := consumer.New(consumeCfg, rssFeedsProcessor, logger)
			if err != nil {
				fmt.Println("FATAL: consumer creation failed, ", err)
				os.Exit(1)
			}
			wrkr := worker.New(consumer, logger)
			wrkr.Start()
		},
	}

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
