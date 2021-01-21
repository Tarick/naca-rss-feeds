package main

//go:generate swagger generate spec --scan-models -o ../../internal/docs/swagger.json

import (
	"fmt"
	"os"

	_ "github.com/Tarick/naca-rss-feeds/internal/docs"
	"github.com/Tarick/naca-rss-feeds/internal/logger/zaplogger"

	"github.com/Tarick/naca-rss-feeds/internal/application/server"
	"github.com/Tarick/naca-rss-feeds/internal/messaging"
	"github.com/Tarick/naca-rss-feeds/internal/messaging/nsqclient/producer"
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
		Run: func(cmd *cobra.Command, args []string) {
			if cfgFile != "" {
				// Use config file from the flag.
				viper.SetConfigFile(cfgFile)
			} else {
				viper.AddConfigPath(".")      // optionally look for config in the working directory
				viper.SetConfigName("config") // name of config file (without extension)
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
			// Create web server
			serverCfg := server.Config{}
			serverViperConfig := viper.Sub("server")
			if err := serverViperConfig.UnmarshalExact(&serverCfg); err != nil {
				fmt.Println("FATAL: failure reading 'server' configuration, ", err)
				os.Exit(1)
			}
			handler := server.NewHandler(logger, tracer, db, rssFeedsUpdateProducer)
			httpServer := server.New(serverCfg, logger, handler)
			httpServer.StartAndServe()
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
