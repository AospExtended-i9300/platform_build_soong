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

package java

import (
	"android/soong/android"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var (
	resourceFiles = []string{
		"res/layout/layout.xml",
		"res/values/strings.xml",
		"res/values-en-rUS/strings.xml",
	}

	compiledResourceFiles = []string{
		"aapt2/res/layout_layout.xml.flat",
		"aapt2/res/values_strings.arsc.flat",
		"aapt2/res/values-en-rUS_strings.arsc.flat",
	}
)

func testAppContext(config android.Config, bp string, fs map[string][]byte) *android.TestContext {
	appFS := map[string][]byte{}
	for k, v := range fs {
		appFS[k] = v
	}

	for _, file := range resourceFiles {
		appFS[file] = nil
	}

	return testContext(config, bp, appFS)
}

func testApp(t *testing.T, bp string) *android.TestContext {
	config := testConfig(nil)

	ctx := testAppContext(config, bp, nil)

	run(t, ctx, config)

	return ctx
}

func TestApp(t *testing.T) {
	for _, moduleType := range []string{"android_app", "android_library"} {
		t.Run(moduleType, func(t *testing.T) {
			ctx := testApp(t, moduleType+` {
					name: "foo",
					srcs: ["a.java"],
				}
			`)

			foo := ctx.ModuleForTests("foo", "android_common")

			var expectedLinkImplicits []string

			manifestFixer := foo.Output("manifest_fixer/AndroidManifest.xml")
			expectedLinkImplicits = append(expectedLinkImplicits, manifestFixer.Output.String())

			frameworkRes := ctx.ModuleForTests("framework-res", "android_common")
			expectedLinkImplicits = append(expectedLinkImplicits,
				frameworkRes.Output("package-res.apk").Output.String())

			// Test the mapping from input files to compiled output file names
			compile := foo.Output(compiledResourceFiles[0])
			if !reflect.DeepEqual(resourceFiles, compile.Inputs.Strings()) {
				t.Errorf("expected aapt2 compile inputs expected:\n  %#v\n got:\n  %#v",
					resourceFiles, compile.Inputs.Strings())
			}

			compiledResourceOutputs := compile.Outputs.Strings()
			sort.Strings(compiledResourceOutputs)

			expectedLinkImplicits = append(expectedLinkImplicits, compiledResourceOutputs...)

			list := foo.Output("aapt2/res.list")
			expectedLinkImplicits = append(expectedLinkImplicits, list.Output.String())

			// Check that the link rule uses
			res := ctx.ModuleForTests("foo", "android_common").Output("package-res.apk")
			if !reflect.DeepEqual(expectedLinkImplicits, res.Implicits.Strings()) {
				t.Errorf("expected aapt2 link implicits expected:\n  %#v\n got:\n  %#v",
					expectedLinkImplicits, res.Implicits.Strings())
			}
		})
	}
}

func TestResourceDirs(t *testing.T) {
	testCases := []struct {
		name      string
		prop      string
		resources []string
	}{
		{
			name:      "no resource_dirs",
			prop:      "",
			resources: []string{"res/res/values/strings.xml"},
		},
		{
			name:      "resource_dirs",
			prop:      `resource_dirs: ["res"]`,
			resources: []string{"res/res/values/strings.xml"},
		},
		{
			name:      "empty resource_dirs",
			prop:      `resource_dirs: []`,
			resources: nil,
		},
	}

	fs := map[string][]byte{
		"res/res/values/strings.xml": nil,
	}

	bp := `
			android_app {
				name: "foo",
				%s
			}
		`

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			config := testConfig(nil)
			ctx := testContext(config, fmt.Sprintf(bp, testCase.prop), fs)
			run(t, ctx, config)

			module := ctx.ModuleForTests("foo", "android_common")
			resourceList := module.MaybeOutput("aapt2/res.list")

			var resources []string
			if resourceList.Rule != nil {
				for _, compiledResource := range resourceList.Inputs.Strings() {
					resources = append(resources, module.Output(compiledResource).Inputs.Strings()...)
				}
			}

			if !reflect.DeepEqual(resources, testCase.resources) {
				t.Errorf("expected resource files %q, got %q",
					testCase.resources, resources)
			}
		})
	}
}

func TestAndroidResources(t *testing.T) {
	testCases := []struct {
		name                       string
		enforceRROTargets          []string
		enforceRROExcludedOverlays []string
		resourceFiles              map[string][]string
		overlayFiles               map[string][]string
		rroDirs                    map[string][]string
	}{
		{
			name:                       "no RRO",
			enforceRROTargets:          nil,
			enforceRROExcludedOverlays: nil,
			resourceFiles: map[string][]string{
				"foo":  nil,
				"bar":  {"bar/res/res/values/strings.xml"},
				"lib":  nil,
				"lib2": {"lib2/res/res/values/strings.xml"},
			},
			overlayFiles: map[string][]string{
				"foo": {
					buildDir + "/.intermediates/lib2/android_common/package-res.apk",
					buildDir + "/.intermediates/lib/android_common/package-res.apk",
					"foo/res/res/values/strings.xml",
					"device/vendor/blah/static_overlay/foo/res/values/strings.xml",
					"device/vendor/blah/overlay/foo/res/values/strings.xml",
				},
				"bar": {
					"device/vendor/blah/static_overlay/bar/res/values/strings.xml",
					"device/vendor/blah/overlay/bar/res/values/strings.xml",
				},
				"lib": {
					buildDir + "/.intermediates/lib2/android_common/package-res.apk",
					"lib/res/res/values/strings.xml",
					"device/vendor/blah/overlay/lib/res/values/strings.xml",
				},
			},
			rroDirs: map[string][]string{
				"foo": nil,
				"bar": nil,
			},
		},
		{
			name:                       "enforce RRO on foo",
			enforceRROTargets:          []string{"foo"},
			enforceRROExcludedOverlays: []string{"device/vendor/blah/static_overlay"},
			resourceFiles: map[string][]string{
				"foo":  nil,
				"bar":  {"bar/res/res/values/strings.xml"},
				"lib":  nil,
				"lib2": {"lib2/res/res/values/strings.xml"},
			},
			overlayFiles: map[string][]string{
				"foo": {
					buildDir + "/.intermediates/lib2/android_common/package-res.apk",
					buildDir + "/.intermediates/lib/android_common/package-res.apk",
					"foo/res/res/values/strings.xml",
					"device/vendor/blah/static_overlay/foo/res/values/strings.xml",
				},
				"bar": {
					"device/vendor/blah/static_overlay/bar/res/values/strings.xml",
					"device/vendor/blah/overlay/bar/res/values/strings.xml",
				},
				"lib": {
					buildDir + "/.intermediates/lib2/android_common/package-res.apk",
					"lib/res/res/values/strings.xml",
					"device/vendor/blah/overlay/lib/res/values/strings.xml",
				},
			},

			rroDirs: map[string][]string{
				"foo": {
					"device/vendor/blah/overlay/foo/res",
					// Enforce RRO on "foo" could imply RRO on static dependencies, but for now it doesn't.
					// "device/vendor/blah/overlay/lib/res",
				},
				"bar": nil,
				"lib": nil,
			},
		},
		{
			name:              "enforce RRO on all",
			enforceRROTargets: []string{"*"},
			enforceRROExcludedOverlays: []string{
				// Excluding specific apps/res directories also allowed.
				"device/vendor/blah/static_overlay/foo",
				"device/vendor/blah/static_overlay/bar/res",
			},
			resourceFiles: map[string][]string{
				"foo":  nil,
				"bar":  {"bar/res/res/values/strings.xml"},
				"lib":  nil,
				"lib2": {"lib2/res/res/values/strings.xml"},
			},
			overlayFiles: map[string][]string{
				"foo": {
					buildDir + "/.intermediates/lib2/android_common/package-res.apk",
					buildDir + "/.intermediates/lib/android_common/package-res.apk",
					"foo/res/res/values/strings.xml",
					"device/vendor/blah/static_overlay/foo/res/values/strings.xml",
				},
				"bar": {"device/vendor/blah/static_overlay/bar/res/values/strings.xml"},
				"lib": {
					buildDir + "/.intermediates/lib2/android_common/package-res.apk",
					"lib/res/res/values/strings.xml",
				},
			},
			rroDirs: map[string][]string{
				"foo": {
					"device/vendor/blah/overlay/foo/res",
					"device/vendor/blah/overlay/lib/res",
				},
				"bar": {"device/vendor/blah/overlay/bar/res"},
				"lib": {"device/vendor/blah/overlay/lib/res"},
			},
		},
	}

	resourceOverlays := []string{
		"device/vendor/blah/overlay",
		"device/vendor/blah/overlay2",
		"device/vendor/blah/static_overlay",
	}

	fs := map[string][]byte{
		"foo/res/res/values/strings.xml":                               nil,
		"bar/res/res/values/strings.xml":                               nil,
		"lib/res/res/values/strings.xml":                               nil,
		"lib2/res/res/values/strings.xml":                              nil,
		"device/vendor/blah/overlay/foo/res/values/strings.xml":        nil,
		"device/vendor/blah/overlay/bar/res/values/strings.xml":        nil,
		"device/vendor/blah/overlay/lib/res/values/strings.xml":        nil,
		"device/vendor/blah/static_overlay/foo/res/values/strings.xml": nil,
		"device/vendor/blah/static_overlay/bar/res/values/strings.xml": nil,
		"device/vendor/blah/overlay2/res/values/strings.xml":           nil,
	}

	bp := `
			android_app {
				name: "foo",
				resource_dirs: ["foo/res"],
				static_libs: ["lib"],
			}

			android_app {
				name: "bar",
				resource_dirs: ["bar/res"],
			}

			android_library {
				name: "lib",
				resource_dirs: ["lib/res"],
				static_libs: ["lib2"],
			}

			android_library {
				name: "lib2",
				resource_dirs: ["lib2/res"],
			}
		`

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			config := testConfig(nil)
			config.TestProductVariables.ResourceOverlays = resourceOverlays
			if testCase.enforceRROTargets != nil {
				config.TestProductVariables.EnforceRROTargets = testCase.enforceRROTargets
			}
			if testCase.enforceRROExcludedOverlays != nil {
				config.TestProductVariables.EnforceRROExcludedOverlays = testCase.enforceRROExcludedOverlays
			}

			ctx := testAppContext(config, bp, fs)
			run(t, ctx, config)

			resourceListToFiles := func(module android.TestingModule, list []string) (files []string) {
				for _, o := range list {
					res := module.MaybeOutput(o)
					if res.Rule != nil {
						// If the overlay is compiled as part of this module (i.e. a .arsc.flat file),
						// verify the inputs to the .arsc.flat rule.
						files = append(files, res.Inputs.Strings()...)
					} else {
						// Otherwise, verify the full path to the output of the other module
						files = append(files, o)
					}
				}
				return files
			}

			getResources := func(moduleName string) (resourceFiles, overlayFiles, rroDirs []string) {
				module := ctx.ModuleForTests(moduleName, "android_common")
				resourceList := module.MaybeOutput("aapt2/res.list")
				if resourceList.Rule != nil {
					resourceFiles = resourceListToFiles(module, resourceList.Inputs.Strings())
				}
				overlayList := module.MaybeOutput("aapt2/overlay.list")
				if overlayList.Rule != nil {
					overlayFiles = resourceListToFiles(module, overlayList.Inputs.Strings())
				}

				rroDirs = module.Module().(AndroidLibraryDependency).ExportedRRODirs().Strings()

				return resourceFiles, overlayFiles, rroDirs
			}

			modules := []string{"foo", "bar", "lib", "lib2"}
			for _, module := range modules {
				resourceFiles, overlayFiles, rroDirs := getResources(module)

				if !reflect.DeepEqual(resourceFiles, testCase.resourceFiles[module]) {
					t.Errorf("expected %s resource files:\n  %#v\n got:\n  %#v",
						module, testCase.resourceFiles[module], resourceFiles)
				}
				if !reflect.DeepEqual(overlayFiles, testCase.overlayFiles[module]) {
					t.Errorf("expected %s overlay files:\n  %#v\n got:\n  %#v",
						module, testCase.overlayFiles[module], overlayFiles)
				}
				if !reflect.DeepEqual(rroDirs, testCase.rroDirs[module]) {
					t.Errorf("expected %s rroDirs:  %#v\n got:\n  %#v",
						module, testCase.rroDirs[module], rroDirs)
				}
			}
		})
	}
}

func TestAppSdkVersion(t *testing.T) {
	testCases := []struct {
		name                  string
		sdkVersion            string
		platformSdkInt        int
		platformSdkCodename   string
		platformSdkFinal      bool
		expectedMinSdkVersion string
	}{
		{
			name:                  "current final SDK",
			sdkVersion:            "current",
			platformSdkInt:        27,
			platformSdkCodename:   "REL",
			platformSdkFinal:      true,
			expectedMinSdkVersion: "27",
		},
		{
			name:                  "current non-final SDK",
			sdkVersion:            "current",
			platformSdkInt:        27,
			platformSdkCodename:   "OMR1",
			platformSdkFinal:      false,
			expectedMinSdkVersion: "OMR1",
		},
		{
			name:                  "default final SDK",
			sdkVersion:            "",
			platformSdkInt:        27,
			platformSdkCodename:   "REL",
			platformSdkFinal:      true,
			expectedMinSdkVersion: "27",
		},
		{
			name:                  "default non-final SDK",
			sdkVersion:            "",
			platformSdkInt:        27,
			platformSdkCodename:   "OMR1",
			platformSdkFinal:      false,
			expectedMinSdkVersion: "OMR1",
		},
		{
			name:                  "14",
			sdkVersion:            "14",
			expectedMinSdkVersion: "14",
		},
	}

	for _, moduleType := range []string{"android_app", "android_library"} {
		for _, test := range testCases {
			t.Run(moduleType+" "+test.name, func(t *testing.T) {
				bp := fmt.Sprintf(`%s {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "%s",
				}`, moduleType, test.sdkVersion)

				config := testConfig(nil)
				config.TestProductVariables.Platform_sdk_version = &test.platformSdkInt
				config.TestProductVariables.Platform_sdk_codename = &test.platformSdkCodename
				config.TestProductVariables.Platform_sdk_final = &test.platformSdkFinal

				ctx := testAppContext(config, bp, nil)

				run(t, ctx, config)

				foo := ctx.ModuleForTests("foo", "android_common")
				link := foo.Output("package-res.apk")
				linkFlags := strings.Split(link.Args["flags"], " ")
				min := android.IndexList("--min-sdk-version", linkFlags)
				target := android.IndexList("--target-sdk-version", linkFlags)

				if min == -1 || target == -1 || min == len(linkFlags)-1 || target == len(linkFlags)-1 {
					t.Fatalf("missing --min-sdk-version or --target-sdk-version in link flags: %q", linkFlags)
				}

				gotMinSdkVersion := linkFlags[min+1]
				gotTargetSdkVersion := linkFlags[target+1]

				if gotMinSdkVersion != test.expectedMinSdkVersion {
					t.Errorf("incorrect --min-sdk-version, expected %q got %q",
						test.expectedMinSdkVersion, gotMinSdkVersion)
				}

				if gotTargetSdkVersion != test.expectedMinSdkVersion {
					t.Errorf("incorrect --target-sdk-version, expected %q got %q",
						test.expectedMinSdkVersion, gotTargetSdkVersion)
				}
			})
		}
	}
}

func TestJNI(t *testing.T) {
	ctx := testJava(t, `
		toolchain_library {
			name: "libcompiler_rt-extras",
			src: "",
		}

		toolchain_library {
			name: "libatomic",
			src: "",
		}

		toolchain_library {
			name: "libgcc",
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.builtins-aarch64-android",
			src: "",
		}

		toolchain_library {
			name: "libclang_rt.builtins-arm-android",
			src: "",
		}

		cc_object {
			name: "crtbegin_so",
			stl: "none",
		}

		cc_object {
			name: "crtend_so",
			stl: "none",
		}

		cc_library {
			name: "libjni",
			system_shared_libs: [],
			stl: "none",
		}

		android_test {
			name: "test",
			no_framework_libs: true,
			jni_libs: ["libjni"],
		}

		android_test {
			name: "test_first",
			no_framework_libs: true,
			compile_multilib: "first",
			jni_libs: ["libjni"],
		}

		android_test {
			name: "test_both",
			no_framework_libs: true,
			compile_multilib: "both",
			jni_libs: ["libjni"],
		}

		android_test {
			name: "test_32",
			no_framework_libs: true,
			compile_multilib: "32",
			jni_libs: ["libjni"],
		}

		android_test {
			name: "test_64",
			no_framework_libs: true,
			compile_multilib: "64",
			jni_libs: ["libjni"],
		}
		`)

	// check the existence of the internal modules
	ctx.ModuleForTests("test", "android_common")
	ctx.ModuleForTests("test_first", "android_common")
	ctx.ModuleForTests("test_both", "android_common")
	ctx.ModuleForTests("test_32", "android_common")
	ctx.ModuleForTests("test_64", "android_common")

	testCases := []struct {
		name string
		abis []string
	}{
		{"test", []string{"arm64-v8a"}},
		{"test_first", []string{"arm64-v8a"}},
		{"test_both", []string{"arm64-v8a", "armeabi-v7a"}},
		{"test_32", []string{"armeabi-v7a"}},
		{"test_64", []string{"arm64-v8a"}},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			app := ctx.ModuleForTests(test.name, "android_common")
			jniLibZip := app.Output("jnilibs.zip")
			var abis []string
			args := strings.Fields(jniLibZip.Args["jarArgs"])
			for i := 0; i < len(args); i++ {
				if args[i] == "-P" {
					abis = append(abis, filepath.Base(args[i+1]))
					i++
				}
			}
			if !reflect.DeepEqual(abis, test.abis) {
				t.Errorf("want abis %v, got %v", test.abis, abis)
			}
		})
	}
}

func TestCertificates(t *testing.T) {
	testCases := []struct {
		name                string
		bp                  string
		certificateOverride string
		expected            string
	}{
		{
			name: "default",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
				}
			`,
			certificateOverride: "",
			expected:            "build/target/product/security/testkey.x509.pem build/target/product/security/testkey.pk8",
		},
		{
			name: "module certificate property",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					certificate: ":new_certificate"
				}

				android_app_certificate {
					name: "new_certificate",
			    certificate: "cert/new_cert",
				}
			`,
			certificateOverride: "",
			expected:            "cert/new_cert.x509.pem cert/new_cert.pk8",
		},
		{
			name: "path certificate property",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					certificate: "expiredkey"
				}
			`,
			certificateOverride: "",
			expected:            "build/target/product/security/expiredkey.x509.pem build/target/product/security/expiredkey.pk8",
		},
		{
			name: "certificate overrides",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					certificate: "expiredkey"
				}

				android_app_certificate {
					name: "new_certificate",
			    certificate: "cert/new_cert",
				}
			`,
			certificateOverride: "foo:new_certificate",
			expected:            "cert/new_cert.x509.pem cert/new_cert.pk8",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			config := testConfig(nil)
			if test.certificateOverride != "" {
				config.TestProductVariables.CertificateOverrides = []string{test.certificateOverride}
			}
			ctx := testAppContext(config, test.bp, nil)

			run(t, ctx, config)
			foo := ctx.ModuleForTests("foo", "android_common")

			signapk := foo.Output("foo.apk")
			signFlags := signapk.Args["certificates"]
			if test.expected != signFlags {
				t.Errorf("Incorrect signing flags, expected: %q, got: %q", test.expected, signFlags)
			}
		})
	}
}

func TestPackageNameOverride(t *testing.T) {
	testCases := []struct {
		name                string
		bp                  string
		packageNameOverride string
		expected            []string
	}{
		{
			name: "default",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
				}
			`,
			packageNameOverride: "",
			expected: []string{
				buildDir + "/.intermediates/foo/android_common/foo.apk",
				buildDir + "/target/product/test_device/system/app/foo/foo.apk",
			},
		},
		{
			name: "overridden",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
				}
			`,
			packageNameOverride: "foo:bar",
			expected: []string{
				// The package apk should be still be the original name for test dependencies.
				buildDir + "/.intermediates/foo/android_common/foo.apk",
				buildDir + "/target/product/test_device/system/app/bar/bar.apk",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			config := testConfig(nil)
			if test.packageNameOverride != "" {
				config.TestProductVariables.PackageNameOverrides = []string{test.packageNameOverride}
			}
			ctx := testAppContext(config, test.bp, nil)

			run(t, ctx, config)
			foo := ctx.ModuleForTests("foo", "android_common")

			outputs := foo.AllOutputs()
			outputMap := make(map[string]bool)
			for _, o := range outputs {
				outputMap[o] = true
			}
			for _, e := range test.expected {
				if _, exist := outputMap[e]; !exist {
					t.Errorf("Can't find %q in output files.\nAll outputs:%v", e, outputs)
				}
			}
		})
	}
}

func TestInstrumentationTargetOverridden(t *testing.T) {
	bp := `
		android_app {
			name: "foo",
			srcs: ["a.java"],
		}

		android_test {
			name: "bar",
			instrumentation_for: "foo",
		}
		`
	config := testConfig(nil)
	config.TestProductVariables.ManifestPackageNameOverrides = []string{"foo:org.dandroid.bp"}
	ctx := testAppContext(config, bp, nil)

	run(t, ctx, config)

	bar := ctx.ModuleForTests("bar", "android_common")
	res := bar.Output("package-res.apk")
	aapt2Flags := res.Args["flags"]
	e := "--rename-instrumentation-target-package org.dandroid.bp"
	if !strings.Contains(aapt2Flags, e) {
		t.Errorf("target package renaming flag, %q is missing in aapt2 link flags, %q", e, aapt2Flags)
	}
}
