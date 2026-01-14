/*
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and admission-webhook-runtime contributors
SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"flag"

	"github.com/go-logr/logr"
	"github.com/sap/admission-webhook-runtime/pkg/admission"
	"k8s.io/apimachinery/pkg/runtime"
)

type GenericWebhook struct{}

func (w *GenericWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	// do the validation (creation case)
	return nil
}

func (w *GenericWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) error {
	// do the validation (update case)
	return nil
}

func (w *GenericWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	// do the validation (deletion case)
	return nil
}

func main() {
	// setup flags
	// replace flag with a more sophisticated module (e.g. pflag) if required
	admission.InitFlags(nil)
	flag.Parse()

	// create and register webhook
	// replace logr.Discard() with a logger of your choice (e.g. klogr, zapr, ...)
	webhook := &GenericWebhook{}
	if err := admission.RegisterValidatingWebhook[runtime.Object](webhook, nil, logr.Discard()); err != nil {
		panic(err)
	}

	// start webhook server
	if err := admission.Serve(context.Background(), nil); err != nil {
		panic(err)
	}
}
