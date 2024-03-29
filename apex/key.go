// Copyright (C) 2018 The Android Open Source Project
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

package apex

import (
	"fmt"
	"io"
	"strings"

	"android/soong/android"

	"github.com/google/blueprint/proptools"
)

var String = proptools.String

func init() {
	android.RegisterModuleType("apex_key", apexKeyFactory)
	android.RegisterSingletonType("apex_keys_text", apexKeysTextFactory)
}

type apexKey struct {
	android.ModuleBase

	properties apexKeyProperties

	public_key_file  android.Path
	private_key_file android.Path

	keyName string
}

type apexKeyProperties struct {
	// Path to the public key file in avbpubkey format. Installed to the device.
	// Base name of the file is used as the ID for the key.
	Public_key *string
	// Path to the private key file in pem format. Used to sign APEXs.
	Private_key *string

	// Whether this key is installable to one of the partitions. Defualt: true.
	Installable *bool
}

func apexKeyFactory() android.Module {
	module := &apexKey{}
	module.AddProperties(&module.properties)
	android.InitAndroidModule(module)
	return module
}

func (m *apexKey) installable() bool {
	return m.properties.Installable == nil || proptools.Bool(m.properties.Installable)
}

func (m *apexKey) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	m.public_key_file = ctx.Config().ApexKeyDir(ctx).Join(ctx, String(m.properties.Public_key))
	m.private_key_file = ctx.Config().ApexKeyDir(ctx).Join(ctx, String(m.properties.Private_key))

	// If not found, fall back to the local key pairs
	if !android.ExistentPathForSource(ctx, m.public_key_file.String()).Valid() {
		m.public_key_file = android.PathForModuleSrc(ctx, String(m.properties.Public_key))
	}
	if !android.ExistentPathForSource(ctx, m.private_key_file.String()).Valid() {
		m.private_key_file = android.PathForModuleSrc(ctx, String(m.properties.Private_key))
	}

	pubKeyName := m.public_key_file.Base()[0 : len(m.public_key_file.Base())-len(m.public_key_file.Ext())]
	privKeyName := m.private_key_file.Base()[0 : len(m.private_key_file.Base())-len(m.private_key_file.Ext())]

	if pubKeyName != privKeyName {
		ctx.ModuleErrorf("public_key %q (keyname:%q) and private_key %q (keyname:%q) do not have same keyname",
			m.public_key_file.String(), pubKeyName, m.private_key_file, privKeyName)
		return
	}
	m.keyName = pubKeyName

	if m.installable() {
		ctx.InstallFile(android.PathForModuleInstall(ctx, "etc/security/apex"), m.keyName, m.public_key_file)
	}
}

func (m *apexKey) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(m.public_key_file),
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				fmt.Fprintln(w, "LOCAL_MODULE_PATH :=", "$(TARGET_OUT)/etc/security/apex")
				fmt.Fprintln(w, "LOCAL_INSTALLED_MODULE_STEM :=", m.keyName)
				fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE :=", !m.installable())
			},
		},
	}
}

////////////////////////////////////////////////////////////////////////
// apex_keys_text
type apexKeysText struct {
	output android.OutputPath
}

func (s *apexKeysText) GenerateBuildActions(ctx android.SingletonContext) {
	s.output = android.PathForOutput(ctx, "apexkeys.txt")
	var filecontent strings.Builder
	ctx.VisitAllModules(func(module android.Module) {
		if m, ok := module.(android.Module); ok && !m.Enabled() {
			return
		}

		if m, ok := module.(*apexBundle); ok {
			fmt.Fprintf(&filecontent,
				"name=%q public_key=%q private_key=%q container_certificate=%q container_private_key=%q\\n",
				m.Name()+".apex",
				m.public_key_file.String(),
				m.private_key_file.String(),
				m.container_certificate_file.String(),
				m.container_private_key_file.String())
		}
	})
	ctx.Build(pctx, android.BuildParams{
		Rule:        android.WriteFile,
		Description: "apexkeys.txt",
		Output:      s.output,
		Args: map[string]string{
			"content": filecontent.String(),
		},
	})
}

func apexKeysTextFactory() android.Singleton {
	return &apexKeysText{}
}

func (s *apexKeysText) MakeVars(ctx android.MakeVarsContext) {
	ctx.Strict("SOONG_APEX_KEYS_FILE", s.output.String())
}
