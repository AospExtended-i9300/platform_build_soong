// Copyright 2019 Google Inc. All rights reserved.
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

package java

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"android/soong/android"
	"android/soong/dexpreopt"
)

func TestDexpreoptBootJars(t *testing.T) {
	bp := `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			installable: true,
		}
	`

	config := testConfig(nil)

	pathCtx := android.PathContextForTesting(config, nil)
	dexpreoptConfig := dexpreopt.GlobalConfigForTests(pathCtx)
	dexpreoptConfig.RuntimeApexJars = []string{"foo", "bar"}
	setDexpreoptTestGlobalConfig(config, dexpreoptConfig)

	ctx := testContext(config, bp, nil)

	ctx.RegisterSingletonType("dex_bootjars", android.SingletonFactoryAdaptor(dexpreoptBootJarsFactory))

	run(t, ctx, config)

	dexpreoptBootJars := ctx.SingletonForTests("dex_bootjars")

	bootArt := dexpreoptBootJars.Output("boot.art")

	expectedInputs := []string{
		"dex_bootjars_input/foo.jar",
		"dex_bootjars_input/bar.jar",
	}

	for i := range expectedInputs {
		expectedInputs[i] = filepath.Join(buildDir, "test_device", expectedInputs[i])
	}

	inputs := bootArt.Implicits.Strings()
	sort.Strings(inputs)
	sort.Strings(expectedInputs)

	if !reflect.DeepEqual(inputs, expectedInputs) {
		t.Errorf("want inputs %q\n got inputs %q", expectedInputs, inputs)
	}

	expectedOutputs := []string{
		"dex_bootjars/system/framework/arm64/boot.invocation",

		"dex_bootjars/system/framework/arm64/boot.art",
		"dex_bootjars/system/framework/arm64/boot-bar.art",

		"dex_bootjars/system/framework/arm64/boot.oat",
		"dex_bootjars/system/framework/arm64/boot-bar.oat",

		"dex_bootjars/system/framework/arm64/boot.vdex",
		"dex_bootjars/system/framework/arm64/boot-bar.vdex",

		"dex_bootjars_unstripped/system/framework/arm64/boot.oat",
		"dex_bootjars_unstripped/system/framework/arm64/boot-bar.oat",
	}

	for i := range expectedOutputs {
		expectedOutputs[i] = filepath.Join(buildDir, "test_device", expectedOutputs[i])
	}

	outputs := bootArt.Outputs.Strings()
	sort.Strings(outputs)
	sort.Strings(expectedOutputs)

	if !reflect.DeepEqual(outputs, expectedOutputs) {
		t.Errorf("want outputs %q\n got outputs %q", expectedOutputs, outputs)
	}
}
