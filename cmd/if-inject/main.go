/*
  SPDX-License-Identifier: CC0-1.0
  https://creativecommons.org/publicdomain/zero/1.0/
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var version = "unknown"
var cmds = map[string]func(ctx context.Context, args []string) int{
	"getnetns": getNetns,
}

func main() {
	showVersion := flag.Bool("version", false, "Swow version and exit")
	lvl := flag.Int("loglevel", 0, "Log level")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	logger := createLogger(*lvl)
	logger.V(1).Info("Start", "version", version)
	ctx := logr.NewContext(context.Background(), logger)
	os.Exit(invokeCmd(ctx, flag.Args()))
}

// subCommand A sub-command
func getNetns(ctx context.Context, args []string) int {
	logger := logr.FromContextOrDiscard(ctx)
	fset := flag.NewFlagSet("", flag.ExitOnError)
	ns := fset.String("ns", "default", "Namespace")
	pod := fset.String("pod", "", "POD")
	logger.V(2).Info("getNetns", "ns", ns, "pod", pod)
	if fset.Parse(args[1:]) != nil {
		return 0
	}
	if *pod == "" {
		logger.Error(fmt.Errorf("Must be specified"), "pod")
		return 1
	}
	return 0
}

// createLogger Create a Zap logger
func createLogger(lvl int) logr.Logger {
	zc := zap.NewProductionConfig()
	zc.Level = zap.NewAtomicLevelAt(zapcore.Level(-lvl))
	zc.DisableStacktrace = true
	zc.DisableCaller = true
	zc.Sampling = nil
	zc.EncoderConfig.TimeKey = "time"
	zc.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	//zc.OutputPaths = []string{"stdout"}
	z, err := zc.Build()
	if err != nil {
		panic(fmt.Sprintf("Can't create a zap logger (%v)?", err))
	}
	return zapr.NewLogger(z)
}

// invokeCmd Invoke a sub-command
func invokeCmd(ctx context.Context, args []string) int {
	logger := logr.FromContextOrDiscard(ctx)
	if len(args) < 1 {
		fmt.Println("Subcommands:")
		for k := range cmds {
			fmt.Println("  ", k)
		}
		return 0
	}

	if cmd, ok := cmds[args[0]]; ok {
		rc := cmd(ctx, args)
		return rc
	}
	logger.Info("Invalid", "command", args[0])
	return -1
}
