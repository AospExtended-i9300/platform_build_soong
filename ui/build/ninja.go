// Copyright 2017 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"android/soong/ui/metrics"
	"android/soong/ui/status"
)

func runNinja(ctx Context, config Config) {
	ctx.BeginTrace(metrics.PrimaryNinja, "ninja")
	defer ctx.EndTrace()

	fifo := filepath.Join(config.OutDir(), ".ninja_fifo")
	status.NinjaReader(ctx, ctx.Status.StartTool(), fifo)

	executable := config.PrebuiltBuildTool("ninja")
	args := []string{
		"-d", "keepdepfile",
		"--frontend_file", fifo,
	}

	args = append(args, config.NinjaArgs()...)

	var parallel int
	if config.UseGoma() {
		parallel = config.RemoteParallel()
	} else {
		parallel = config.Parallel()
	}
	args = append(args, "-j", strconv.Itoa(parallel))
	if config.keepGoing != 1 {
		args = append(args, "-k", strconv.Itoa(config.keepGoing))
	}

	args = append(args, "-f", config.CombinedNinjaFile())

	args = append(args,
		"-w", "dupbuild=err",
		"-w", "missingdepfile=err")

	cmd := Command(ctx, config, "ninja", executable, args...)
	cmd.Sandbox = ninjaSandbox
	if config.HasKatiSuffix() {
		cmd.Environment.AppendFromKati(config.KatiEnvFile())
	}

	cmd.Environment.Set("DIST_DIR", config.DistDir())

	// Allow both NINJA_ARGS and NINJA_EXTRA_ARGS, since both have been
	// used in the past to specify extra ninja arguments.
	if extra, ok := cmd.Environment.Get("NINJA_ARGS"); ok {
		cmd.Args = append(cmd.Args, strings.Fields(extra)...)
	}
	if extra, ok := cmd.Environment.Get("NINJA_EXTRA_ARGS"); ok {
		cmd.Args = append(cmd.Args, strings.Fields(extra)...)
	}

	logPath := filepath.Join(config.OutDir(), ".ninja_log")
	ninjaHeartbeatDuration := time.Minute * 5
	if overrideText, ok := cmd.Environment.Get("NINJA_HEARTBEAT_INTERVAL"); ok {
		// For example, "1m"
		overrideDuration, err := time.ParseDuration(overrideText)
		if err == nil && overrideDuration.Seconds() > 0 {
			ninjaHeartbeatDuration = overrideDuration
		}
	}
	// Poll the ninja log for updates; if it isn't updated enough, then we want to show some diagnostics
	done := make(chan struct{})
	defer close(done)
	ticker := time.NewTicker(ninjaHeartbeatDuration)
	defer ticker.Stop()
	checker := &statusChecker{}
	go func() {
		for {
			select {
			case <-ticker.C:
				checker.check(ctx, config, logPath)
			case <-done:
				return
			}
		}
	}()

	ctx.Status.Status("Starting ninja...")
	cmd.RunAndPrintOrFatal()
}

type statusChecker struct {
	prevTime time.Time
}

func (c *statusChecker) check(ctx Context, config Config, pathToCheck string) {
	info, err := os.Stat(pathToCheck)
	var newTime time.Time
	if err == nil {
		newTime = info.ModTime()
	}
	if newTime == c.prevTime {
		// ninja may be stuck
		dumpStucknessDiagnostics(ctx, config, pathToCheck, newTime)
	}
	c.prevTime = newTime
}

// dumpStucknessDiagnostics gets called when it is suspected that Ninja is stuck and we want to output some diagnostics
func dumpStucknessDiagnostics(ctx Context, config Config, statusPath string, lastUpdated time.Time) {

	ctx.Verbosef("ninja may be stuck; last update to %v was %v. dumping process tree...", statusPath, lastUpdated)

	// The "pstree" command doesn't exist on Mac, but "pstree" on Linux gives more convenient output than "ps"
	// So, we try pstree first, and ps second
	pstreeCommandText := fmt.Sprintf("pstree -pal %v", os.Getpid())
	psCommandText := "ps -ef"
	commandText := pstreeCommandText + " || " + psCommandText

	cmd := Command(ctx, config, "dump process tree", "bash", "-c", commandText)
	output := cmd.CombinedOutputOrFatal()
	ctx.Verbose(string(output))

	ctx.Verbosef("done\n")
}
