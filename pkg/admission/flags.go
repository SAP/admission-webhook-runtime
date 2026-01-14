/*
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and admission-webhook-runtime contributors
SPDX-License-Identifier: Apache-2.0
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
