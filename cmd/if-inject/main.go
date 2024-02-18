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
	"github.com/uablrek/if-inject/pkg/util"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types"
)

var version = "unknown"
var cmds = map[string]func(ctx context.Context, args []string) int{
	"netns": getNetns,
	"add": add,
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

// getNetns Print the netns
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
	podObj, err := util.GetPOD(ctx, *ns, *pod)
	if err != nil {
		logger.Error(err, "GetPOD")
		return 1
	}
	runtime, err := util.NewRuntimeConnection(ctx, "")
	if err != nil {
		logger.Error(err, "RuntimeConnection")
		return 1
	}
	netns, _, err := runtime.GetNetns(ctx, podObj)
	if err != nil {
		logger.Error(err, "GetNetns")
		return 1
	}
	fmt.Println(netns)
	return 0
}

// add Add a network
func add(ctx context.Context, args []string) int {
	return invokeCni(ctx, "add", args)
}

// invokeCni Invoke an operation over the CNI
func invokeCni(ctx context.Context, op string, args []string) int {
	logger := logr.FromContextOrDiscard(ctx)
	fset := flag.NewFlagSet("", flag.ExitOnError)
	ns := fset.String("ns", "default", "Namespace")
	pod := fset.String("pod", "", "POD")
	iface := fset.String("interface", "net1", "Interface name in the POD")
	spec := fset.String("spec", "", "The CNI spec (file)")
	logger.V(2).Info(
		"invokeCni", "ns", ns, "pod", pod, "interface", iface, "spec", spec)
	if fset.Parse(args[1:]) != nil {
		return 0
	}
	if *pod == "" {
		logger.Error(fmt.Errorf("Must be specified"), "pod")
		return 1
	}
	if *spec == "" {
		logger.Error(fmt.Errorf("Must be specified"), "spec")
		return 1
	}

	podObj, err := util.GetPOD(ctx, *ns, *pod)
	if err != nil {
		logger.Error(err, "GetPOD")
		return 1
	}
	runtime, err := util.NewRuntimeConnection(ctx, "")
	if err != nil {
		logger.Error(err, "RuntimeConnection")
		return 1
	}
	netns, containerID, err := runtime.GetNetns(ctx, podObj)
	if err != nil {
		logger.Error(err, "GetNetns")
		return 1
	}

	netconf, err := libcni.ConfFromFile(*spec)
	if err != nil {
		logger.Error(err, "read spec")
		return 1
	}

	cninet := libcni.NewCNIConfig([]string{"/opt/cni/bin"}, nil)
	
	rt := &libcni.RuntimeConf{
		ContainerID:    containerID,
		NetNS:          netns,
		IfName:         *iface,
	}

	logger.V(1).Info(
		"invoke CNI", "op", op, "cninet", cninet, "netconf", netconf, "rt", rt)
	switch op {
	case "add":
		var result types.Result
		result, err = cninet.AddNetwork(ctx, netconf, rt)
		if result != nil {
			_ = result.Print()
		}
	}
	if err != nil {
		logger.Error(err, "invoke CNI", "op", op)
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
