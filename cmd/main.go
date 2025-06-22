package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/brettbedarf/webfs"
	"github.com/brettbedarf/webfs/adapters"
	"github.com/brettbedarf/webfs/config"
	"github.com/brettbedarf/webfs/fs"
	"github.com/brettbedarf/webfs/requests"
	"github.com/brettbedarf/webfs/util"
)

func main() {
	// Parse command line arguments
	var (
		// configPath string
		debug    bool
		nodesDef string
	)
	// configPath := flag.String("config", "", "Path to config file")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.BoolVar(&debug, "d", false, "Enable debug logging (shorthand)")
	flag.StringVar(&nodesDef, "nodes", "", "Path to nodes def file")
	flag.StringVar(&nodesDef, "n", "", "Path to nodes def file (shorthand")
	flag.Parse()
	mnt := flag.Arg(0)

	// Initialize logger
	logLevel := util.InfoLevel
	if debug {
		logLevel = util.DebugLevel
	}
	util.InitializeLogger(logLevel)
	logger := util.GetLogger("main")

	logger.Debug().Bool("debug", debug).Str("nodes", nodesDef).Str("mnt", mnt).Msg("CLI Args")
	// Check if mount point is provided
	if mnt == "" {
		logger.Fatal().Msg("Mount point not specified; it must be passed as the argument")
	}

	// Register all built-in adapters
	adapters.RegisterBuiltins()

	// Init the fs
	cfg := config.NewDefaultConfig()
	webfs := webfs.New(cfg)

	// Load sources
	if nodesDef != "" {
		defData, err := os.ReadFile(nodesDef)
		if err != nil {
			logger.Fatal().Err(err).Str("sources", nodesDef).Msg("Failed to read sources file")
		}
		logger.Debug().Str("sources", nodesDef).Msg("Sources file loaded successfully")
		var rawNodes []json.RawMessage
		if err := json.Unmarshal(defData, &rawNodes); err != nil {
			logger.Error().Err(err).Msg("Failed to unmarshal sources")
		}

		var fileRequests []*fs.FileCreateRequest
		var dirRequests []*fs.DirCreateRequest

		for _, rawNode := range rawNodes {
			// Determine the node type
			nodeType, err := requests.GetNodeType(rawNode)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to get node type")
				continue
			}

			switch nodeType {
			case fs.FileNodeType:
				fileReq, err := requests.UnmarshalFileRequest(rawNode)
				if err != nil {
					logger.Error().Err(err).Msg("Failed to unmarshal file request")
					continue
				}
				fileRequests = append(fileRequests, fileReq)
				logger.Debug().Str("path", fileReq.Path).Msg("Processed file request")

			case fs.DirNodeType:
				dirReq, err := requests.UnmarshalDirRequest(rawNode)
				if err != nil {
					logger.Error().Err(err).Msg("Failed to unmarshal directory request")
					continue
				}
				dirRequests = append(dirRequests, dirReq)
				logger.Debug().Str("path", dirReq.Path).Msg("Processed directory request")

			default:
				logger.Warn().Str("type", string(nodeType)).Msg("Unknown node type")
			}
		}

		logger.Info().
			Int("files", len(fileRequests)).
			Int("directories", len(dirRequests)).
			Msg("Successfully processed source requests")

		// TODO: Add nodes to webfs
	} else {
		logger.Warn().Msg("No sources file provided")
	}

	// Serve
	if err := webfs.Serve(mnt); err != nil {
		logger.Fatal().Err(err).Msg("Failed to mount filesystem")
	}

	// Setup signal handling for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info().Str("mountpoint", mnt).Msg("Filesystem mounted successfully")

	// Wait for termination signal
	sig := <-signalChan
	logger.Info().Str("signal", sig.String()).Msg("Received signal, unmounting filesystem")

	// Unmount the filesystem
	if err := webfs.Unmount(); err != nil {
		logger.Error().Err(err).Msg("Failed to unmount filesystem")
	} else {
		logger.Info().Msg("Filesystem unmounted successfully")
	}
}
