package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/brettbedarf/webfs"
	"github.com/brettbedarf/webfs/adapters"
	"github.com/brettbedarf/webfs/config"
	"github.com/brettbedarf/webfs/internal/util"
	"github.com/brettbedarf/webfs/requests"
	"github.com/brettbedarf/webfs/server"
)

func main() {
	// Parse command line arguments
	var (
		// configPath string
		verbose  int
		nodesDef string
		umount   bool
	)
	// configPath := flag.String("config", "", "Path to config file")
	flag.StringVar(&nodesDef, "nodes", "", "Path to nodes def file")
	flag.StringVar(&nodesDef, "n", "", "--nodes (shorthand)")
	flag.BoolVar(&umount, "umount", false,
		"Unmount the fs first if needed before mounting again. Useful for debuggers that don't exit properly.")
	flag.BoolVar(&umount, "u", false, "--umount (shorthand)")
	flag.IntVar(&verbose, "verbose", 3, "Log verbosity level between 1 (error) and 5 (trace). Default is 3 (info).")
	flag.IntVar(&verbose, "v", 3, "--verbose (shorthand)")
	flag.Parse()

	// Initialize logger
	if verbose < 1 {
		verbose = 1
	}
	if verbose > 5 {
		verbose = 5
	}
	logLvls := [5]util.LogLevel{util.ErrorLevel, util.WarnLevel, util.InfoLevel, util.DebugLevel, util.TraceLevel}
	logLvl := logLvls[verbose-1]
	util.InitializeLogger(logLvl)
	logger := util.GetLogger("main")

	mnt := flag.Arg(0)
	logger.Info().Int("verbose", verbose).Str("nodes", nodesDef).Str("mnt", mnt).Msg("WebFS server initializing")
	// Check if mount point is provided
	if mnt == "" {
		logger.Fatal().Msg("Mount point not specified; it must be passed as the argument")
	}
	// Try unmount if requested
	if umount { // send cli command
		cmd := exec.Command("fusermount", "-u", mnt)
		// we ignore error here if not already mounted
		cmd.Run() // nolint:errcheck
	}
	// Register all built-in adapters
	adapters.RegisterBuiltins()

	// Init the fs
	cfg := config.NewConfig(&config.ConfigOverride{
		LogLvl: &logLvl,
	})

	fs := server.New(cfg)
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

		var fileRequests []*webfs.FileCreateRequest
		var dirRequests []*webfs.DirCreateRequest

		for _, rawNode := range rawNodes {
			// Determine the node type
			nodeType, err := requests.GetNodeType(rawNode)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to get node type")
				continue
			}

			switch nodeType {
			case webfs.FileNodeType:
				fileReq, err := requests.UnmarshalFileRequest(rawNode)
				if err != nil {
					logger.Error().Err(err).Msg("Failed to unmarshal file request")
					continue
				}
				fileRequests = append(fileRequests, fileReq)
				logger.Debug().Str("path", fileReq.Path).Msg("Processed file request")

			case webfs.DirNodeType:
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
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

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
