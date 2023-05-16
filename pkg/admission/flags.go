/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package admission

import "flag"

var (
	commandLine      flag.FlagSet
	optionsFromFlags ServeOptions
)

// Add our flags to specified go flag set.
// If flagset is nil, the default flag set will be used.
func InitFlags(flagset *flag.FlagSet) {
	if flagset == nil {
		flagset = flag.CommandLine
	}

	commandLine.VisitAll(func(f *flag.Flag) {
		flagset.Var(f.Value, f.Name, f.Usage)
	})
}

// Get our flags as a go flag set.
func FlagSet() *flag.FlagSet {
	flagset := &flag.FlagSet{}
	commandLine.VisitAll(func(f *flag.Flag) {
		flagset.Var(f.Value, f.Name, f.Usage)
	})
	return flagset
}

func init() {
	optionsFromFlags.BindAddress = ":2443"
	commandLine.StringVar(&optionsFromFlags.BindAddress, "bind-address", optionsFromFlags.BindAddress, "Bind address used by the webhook")
	commandLine.StringVar(&optionsFromFlags.CertFile, "tls-cert-file", optionsFromFlags.CertFile, "File containing the default x509 Certificate for https (CA cert, if any, concatenated after server cert)")
	commandLine.StringVar(&optionsFromFlags.KeyFile, "tls-key-file", optionsFromFlags.KeyFile, "File containing the default x509 key matching --tls-cert-file")
}
