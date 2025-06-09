package main

import (
	"flag"

	"github.com/brettbedarf/webfs/util"
)

func main() {
	// Parse command line arguments
	// configPath := flag.String("config", "", "Path to config file")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()
	mountPoint := flag.Arg(0)

	// Initialize logger
	logLevel := util.InfoLevel
	if *debug {
		logLevel = util.DebugLevel
	}
	util.InitializeLogger(logLevel)
	logger := util.GetLogger("main")

	// Check if mount point is provided
	if mountPoint == "" {
		logger.Fatal().Msg("Mount point not specified; it must be passed as the argument")
	}

	//	fs := filesystem.newFsTree()
	//
	//	// Create filesystem
	//	fusefs := filesystem.NewFuseRaw(mountPoint, &MountOptions{
	//		FsName: "httpfs",
	//		Name:   "httpfs",
	//		Debug:  *debug,
	//		Logger: log.Default(),
	//	}
	//
	// )
	//
	//	server, err := NewServer(fusefs, mountPoint, opts)
	//	if err != nil {
	//		logger.Fatal().Err(err).Msg("Failed to create FUSE server")
	//	}
	//
	//	// Setup signal handling for graceful shutdown
	//	signalChan := make(chan os.Signal, 1)
	//	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	//
	//	// Start server in the background
	//	go server.Serve()
	//
	//	// Wait until the filesystem is mounted
	//	if err := server.WaitMount(); err != nil {
	//		logger.Fatal().Err(err).Msg("Failed to mount filesystem")
	//	}
	//
	//	logger.Info().Str("mountpoint", mountPoint).Msg("Filesystem mounted successfully")
	//
	//	// Wait for termination signal
	//	sig := <-signalChan
	//	logger.Info().Str("signal", sig.String()).Msg("Received signal, unmounting filesystem")
	//
	//	// Unmount the filesystem
	//	if err := server.Unmount(); err != nil {
	//		logger.Error().Err(err).Msg("Failed to unmount filesystem")
	//	}
	//
	//	logger.Info().Msg("Filesystem unmounted successfully")
}
