/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and admission-webhook-runtime contributors
SPDX-License-Identifier: Apache-2.0
*/

package admission

import (
	"context"
	"fmt"
	"reflect"

	"github.com/sap/go-generics/slices"

	"k8s.io/apimachinery/pkg/runtime"
)

type MultiValidatingWebhook[T runtime.Object] struct {
	webhooks []ValidatingWebhook[T]
}

var _ ValidatingWebhook[runtime.Object] = &MultiValidatingWebhook[runtime.Object]{}

func NewMultiValidatingWebhook[T runtime.Object](webhooks ...ValidatingWebhook[T]) *MultiValidatingWebhook[T] {
	return &MultiValidatingWebhook[T]{
		webhooks: webhooks,
	}
}

func (w *MultiValidatingWebhook[T]) ValidateCreate(ctx context.Context, obj T) error {
	for _, w := range w.webhooks {
		if err := w.ValidateCreate(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}

func (w *MultiValidatingWebhook[T]) ValidateUpdate(ctx context.Context, oldObj T, newObj T) error {
	for _, w := range w.webhooks {
		if err := w.ValidateUpdate(ctx, oldObj, newObj); err != nil {
			return err
		}
	}
	return nil
}

func (w *MultiValidatingWebhook[T]) ValidateDelete(ctx context.Context, obj T) error {
	for _, w := range w.webhooks {
		if err := w.ValidateDelete(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}

type MultiMutatingWebhook[T runtime.Object] struct {
	webhooks []MutatingWebhook[T]
}

var _ MutatingWebhook[runtime.Object] = &MultiMutatingWebhook[runtime.Object]{}

func NewMultiMutatingWebhook[T runtime.Object](webhooks ...MutatingWebhook[T]) *MultiMutatingWebhook[T] {
	return &MultiMutatingWebhook[T]{
		webhooks: webhooks,
	}
}

func (w *MultiMutatingWebhook[T]) MutateCreate(ctx context.Context, obj T) error {
	for i := 0; ; i++ {
		if i > 100*len(w.webhooks) {
			return fmt.Errorf("potential endless mutation loop detected")
		}
		count := 0
		savedObj := obj.DeepCopyObject().(T)
		for _, w := range w.webhooks {
			if err := w.MutateCreate(ctx, obj); err != nil {
				return err
			}
			if !reflect.DeepEqual(obj, savedObj) {
				count++
			}
			if count > 1 {
				break
			}
		}
		if count <= 1 {
			break
		}
	}
	return nil
}

func (w *MultiMutatingWebhook[T]) MutateUpdate(ctx context.Context, oldObj T, newObj T) error {
	for i := 0; ; i++ {
		if i > 100*len(w.webhooks) {
			return fmt.Errorf("potential endless mutation loop detected")
		}
		count := 0
		savedObj := newObj.DeepCopyObject().(T)
		for _, w := range w.webhooks {
			if err := w.MutateUpdate(ctx, oldObj, newObj); err != nil {
				return err
			}
			if !reflect.DeepEqual(newObj, savedObj) {
				count++
			}
			if count > 1 {
				break
			}
		}
		if count <= 1 {
			break
		}
	}
	return nil
}

type MultiWebhook[T runtime.Object] struct {
	MultiValidatingWebhook[T]
	MultiMutatingWebhook[T]
}

var _ Webhook[runtime.Object] = &MultiWebhook[runtime.Object]{}

func NewMultiWebhook[T runtime.Object](webhooks ...Webhook[T]) *MultiWebhook[T] {
	return &MultiWebhook[T]{
		MultiValidatingWebhook: MultiValidatingWebhook[T]{webhooks: slices.Collect(webhooks, func(w Webhook[T]) ValidatingWebhook[T] { return w })},
		MultiMutatingWebhook:   MultiMutatingWebhook[T]{webhooks: slices.Collect(webhooks, func(w Webhook[T]) MutatingWebhook[T] { return w })},
	}
}
