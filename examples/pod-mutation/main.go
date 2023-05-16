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

package main

import (
	"context"
	"flag"

	"github.com/go-logr/logr"
	"github.com/sap/admission-webhook-runtime/pkg/admission"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type PodWebhook struct{}

func (w *PodWebhook) MutateCreate(ctx context.Context, pod *corev1.Pod) error {
	// do the mutation (creation case)
	return nil
}

func (w *PodWebhook) MutateUpdate(ctx context.Context, oldPod *corev1.Pod, newPod *corev1.Pod) error {
	// do the mutation (update case)
	return nil
}

func main() {
	// setup flags
	// replace flag with a more sophisticated module (e.g. pflag) if required
	admission.InitFlags(nil)
	flag.Parse()

	// prepare scheme (must know all resource types managed by this webhook)
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(err)
	}

	// create and register webhook
	// replace logr.Discard() with a logger of your choice (e.g. klogr, zapr, ...)
	webhook := &PodWebhook{}
	if err := admission.RegisterMutatingWebhook[*corev1.Pod](webhook, scheme, logr.Discard()); err != nil {
		panic(err)
	}

	// start webhook server
	if err := admission.Serve(context.Background(), nil); err != nil {
		panic(err)
	}
}
