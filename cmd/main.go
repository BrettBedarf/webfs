package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/brettbedarf/webfs"
	"github.com/brettbedarf/webfs/adapters"
	"github.com/brettbedarf/webfs/api"
	"github.com/brettbedarf/webfs/config"
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

	logger.Info().Bool("debug", debug).Str("nodes", nodesDef).Str("mnt", mnt).Msg("WebFS server initializing")
	// Check if mount point is provided
	if mnt == "" {
		logger.Fatal().Msg("Mount point not specified; it must be passed as the argument")
	}

	// Register all built-in adapters
	adapters.RegisterBuiltins()

	// Init the fs
	cfg := config.NewConfig(&config.ConfigOverride{
		Debug: &debug,
	})

	fs := webfs.New(cfg)
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

		var fileRequests []*api.FileCreateRequest
		var dirRequests []*api.DirCreateRequest

		for _, rawNode := range rawNodes {
			// Determine the node type
			nodeType, err := requests.GetNodeType(rawNode)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to get node type")
				continue
			}

			switch nodeType {
			case api.FileNodeType:
				fileReq, err := requests.UnmarshalFileRequest(rawNode)
				if err != nil {
					logger.Error().Err(err).Msg("Failed to unmarshal file request")
					continue
				}
				fileRequests = append(fileRequests, fileReq)
				logger.Debug().Str("path", fileReq.Path).Msg("Processed file request")

			case api.DirNodeType:
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

		logger.Debug().
			Int("files", len(fileRequests)).
			Int("directories", len(dirRequests)).
			Msg("Successfully loaded source requests")

		// Add nodes to webfs
		dirAddCnt := 0
		for _, req := range dirRequests {
			if _, err := fs.AddDirNode(req); err != nil {
				logger.Debug().Interface("request", req).Err(err).Msg("Failed to add directory request")
			}
			dirAddCnt++
		}
		fileAddCnt := 0
		for _, req := range fileRequests {
			if _, err := fs.AddFileNode(req); err != nil {
				logger.Debug().Interface("request", req).Err(err).Msg("Failed to add file request")
			} else {
				fileAddCnt++
			}
		}
		logger.Info().Int("directories", dirAddCnt).Int("files", fileAddCnt).Msg("Added new nodes to filesystem")
	} else {
		logger.Warn().Msg("No sources file provided")
	}

	// Serve
	if err := fs.Serve(mnt); err != nil {
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
	if err := fs.Unmount(); err != nil {
		logger.Error().Err(err).Msg("Failed to unmount filesystem")
	} else {
		logger.Info().Msg("Filesystem unmounted successfully")
	}
}
