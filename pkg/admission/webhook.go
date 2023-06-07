/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and redis-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	jsonpatch "gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// Validating webhook interface.
type ValidatingWebhook[T runtime.Object] interface {
	ValidateCreate(ctx context.Context, obj T) error
	ValidateUpdate(ctx context.Context, oldObj T, newObj T) error
	ValidateDelete(ctx context.Context, obj T) error
}

// Mutating webhook interface.
// There is no deletion handler because mutating before deletion is meaningless anyway.
type MutatingWebhook[T runtime.Object] interface {
	MutateCreate(ctx context.Context, obj T) error
	MutateUpdate(ctx context.Context, oldObj T, newObj T) error
}

// Joint interface for a webhook which is both validating and mutating (for convenience).
type Webhook[T runtime.Object] interface {
	ValidatingWebhook[T]
	MutatingWebhook[T]
}

// todo: safeguard concurrent invocations
// in particular, prevent that Register* is called after Serve is called, and that serve is called more than once

// todo: ensure that webhook registration fails if there is already a webhook registered on a certain path

// todo: currently errors returned from the webhook implementation are always wrapped into a 'forbidden' response;
// we should allow implementations to influence the status in the admission response;
// either by checking if the returned error is a http status error (or - maybe better) by doing that with an
// own error type modeling the http status

// Webhook handler. Implements the http.Handler interface.
type WebhookHandler struct {
	admitFunc func(log logr.Logger, ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse
	log       logr.Logger
}

// Serve admission http request.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handleAdmission(w, r, h.admitFunc, h.log)
}

// Create webhook handler for a validating webhook.
// The type parameter T can be a pointer to a concrete Kubernetes resource type (such as *corev1.Pod),
// a pointer to unstructured.Unstructured, or an interface type containing runtime.Object;
// in the first case, scheme is required and must recognize the supplied resource type; in the second and third case,
// scheme is ignored (can be passed as nil), and a pointer to unstructured.Unstructured will be passed to
// the webhook implementation.
func NewValidatingWebhookHandler[T runtime.Object](w ValidatingWebhook[T], scheme *runtime.Scheme, log logr.Logger) *WebhookHandler {
	var decoder runtime.Decoder
	if scheme == nil {
		decoder = unstructured.UnstructuredJSONScheme
	} else {
		decoder = serializer.NewCodecFactory(scheme).UniversalDeserializer()
	}

	return &WebhookHandler{
		admitFunc: func(log logr.Logger, ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
			var obj, oldObj T
			if len(req.Object.Raw) > 0 {
				object, _, err := decoder.Decode(req.Object.Raw, nil, nil)
				if err != nil {
					return toAdmissionError(http.StatusBadRequest, errors.Wrap(err, "error decoding object from admission request"))
				}
				var ok bool
				if obj, ok = object.(T); !ok {
					return toAdmissionError(http.StatusBadRequest, fmt.Errorf("error converting object from admission request to %T", obj))
				}
			}
			if len(req.OldObject.Raw) > 0 {
				object, _, err := decoder.Decode(req.OldObject.Raw, nil, nil)
				if err != nil {
					return toAdmissionError(http.StatusBadRequest, errors.Wrap(err, "error decoding old object from admission request"))
				}
				var ok bool
				if oldObj, ok = object.(T); !ok {
					return toAdmissionError(http.StatusBadRequest, fmt.Errorf("error converting old object from admission request to %T", oldObj))
				}
			}

			switch req.Operation {
			case admissionv1.Create:
				log.V(2).Info("invoking ValidateCreate")
				if err := w.ValidateCreate(ctx, obj); err != nil {
					return toAdmissionError(http.StatusForbidden, err)
				}
			case admissionv1.Update:
				log.V(2).Info("invoking ValidateUpdate")
				if err := w.ValidateUpdate(ctx, oldObj, obj); err != nil {
					return toAdmissionError(http.StatusForbidden, err)
				}
			case admissionv1.Delete:
				log.V(2).Info("invoking ValidateDelete")
				if err := w.ValidateDelete(ctx, oldObj); err != nil {
					return toAdmissionError(http.StatusForbidden, err)
				}
			}

			return &admissionv1.AdmissionResponse{
				// todo: add Result
				Allowed: true,
			}
		},
		log: log,
	}
}

// Register validating webhook with router (such as http.ServeMux or gorilla's mux.Router).
// The type parameter T can be a pointer to a concrete Kubernetes resource type (such as *corev1.Pod),
// a pointer to unstructured.Unstructured, or an interface type containing runtime.Object;
// in the first case, scheme is required and must recognize the supplied resource type; in the second and third case,
// scheme is ignored (can be passed as nil), and a pointer to unstructured.Unstructured will be passed to
// the webhook implementation.
func RegisterValidatingWebhookWithRouter[T runtime.Object](w ValidatingWebhook[T], scheme *runtime.Scheme, log logr.Logger, router Router) error {
	var obj T
	objType := reflect.TypeOf(obj)
	if objType == nil || objType.Kind() == reflect.Interface {
		log.Info("registering generic validation webhook")

		path := "/generic/validate"
		log.V(1).Info("starting handler", "path", path)
		router.Handle(path, NewValidatingWebhookHandler(w, scheme, log.WithValues("type", "generic validation")))
	} else if objType.Kind() == reflect.Pointer {
		obj = reflect.New(objType.Elem()).Interface().(T)

		if _, ok := any(obj).(*unstructured.Unstructured); ok {
			log.Info("registering generic validation webhook")

			path := "/generic/validate"
			log.V(1).Info("starting handler", "path", path)
			router.Handle(path, NewValidatingWebhookHandler(w, scheme, log.WithValues("type", "generic validation")))
		} else {
			log.Info("registering validation webhook", "type", fmt.Sprintf("%T", obj))

			if scheme == nil {
				return fmt.Errorf("encountering empty/missing scheme")
			}
			gvks, unversioned, err := scheme.ObjectKinds(obj)
			if err != nil {
				return errors.Wrapf(err, "error fetching scheme information for type %T", obj)
			}
			if unversioned {
				return fmt.Errorf("encountering unversioned object type %T; unversioned types are not supported", obj)
			}

			for _, gvk := range gvks {
				if gvk.Group == "" {
					gvk.Group = "core"
				}
				path := "/" + strings.ToLower(gvk.Group) + "/" + strings.ToLower(gvk.Version) + "/" + strings.ToLower(gvk.Kind) + "/validate"
				log.V(1).Info("starting handler", "path", path)
				router.Handle(path, NewValidatingWebhookHandler(w, scheme, log.WithValues("group", gvk.Group, "version", gvk.Version, "kind", gvk.Kind, "type", "validation")))
			}
		}
	} else {
		return fmt.Errorf("encountering unsupported object kind %s", objType.Kind())
	}

	return nil
}

// Register validating webhook to be served by Serve().
// Must be called before Serve().
// The type parameter T can be a pointer to a concrete Kubernetes resource type (such as *corev1.Pod),
// a pointer to unstructured.Unstructured, or an interface type containing runtime.Object;
// in the first case, scheme is required and must recognize the supplied resource type; in the second and third case,
// scheme is ignored (can be passed as nil), and a pointer to unstructured.Unstructured will be passed to
// the webhook implementation.
func RegisterValidatingWebhook[T runtime.Object](w ValidatingWebhook[T], scheme *runtime.Scheme, log logr.Logger) error {
	return RegisterValidatingWebhookWithRouter(w, scheme, log, http.DefaultServeMux)
}

// Create webhook handler for a mutating webhook.
// The type parameter T can be a pointer to a concrete Kubernetes resource type (such as *corev1.Pod),
// a pointer to unstructured.Unstructured, or an interface type containing runtime.Object;
// in the first case, scheme is required and must recognize the supplied resource type; in the second and third case,
// scheme is ignored (can be passed as nil), and a pointer to unstructured.Unstructured will be passed to
// the webhook implementation.
func NewMutatingWebhookHandler[T runtime.Object](w MutatingWebhook[T], scheme *runtime.Scheme, log logr.Logger) *WebhookHandler {
	var decoder runtime.Decoder
	if scheme == nil {
		decoder = unstructured.UnstructuredJSONScheme
	} else {
		decoder = serializer.NewCodecFactory(scheme).UniversalDeserializer()
	}

	return &WebhookHandler{
		admitFunc: func(log logr.Logger, ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
			var obj, oldObj T
			if len(req.Object.Raw) > 0 {
				object, _, err := decoder.Decode(req.Object.Raw, nil, nil)
				if err != nil {
					return toAdmissionError(http.StatusBadRequest, errors.Wrap(err, "error decoding object from admission request"))
				}
				var ok bool
				if obj, ok = object.(T); !ok {
					return toAdmissionError(http.StatusBadRequest, fmt.Errorf("error converting object from admission request to %T", obj))
				}
			}
			if len(req.OldObject.Raw) > 0 {
				object, _, err := decoder.Decode(req.OldObject.Raw, nil, nil)
				if err != nil {
					return toAdmissionError(http.StatusBadRequest, errors.Wrap(err, "error decoding old object from admission request"))
				}
				var ok bool
				if oldObj, ok = object.(T); !ok {
					return toAdmissionError(http.StatusBadRequest, fmt.Errorf("error converting old object from admission request to %T", oldObj))
				}
			}

			switch req.Operation {
			case admissionv1.Create:
				log.V(2).Info("invoking MutateCreate")
				if err := w.MutateCreate(ctx, obj); err != nil {
					return toAdmissionError(http.StatusForbidden, err)
				}
			case admissionv1.Update:
				log.V(2).Info("invoking MutateUpdate")
				if err := w.MutateUpdate(ctx, oldObj, obj); err != nil {
					return toAdmissionError(http.StatusForbidden, err)
				}
			}

			raw := jsonEncode(obj)
			// todo: are we actually sure that req.Object.Raw is guaranteed to be json-encoded ?
			// otherwise we should clone (DeepCopyObject) obj first and re-encode here as well ...
			patches, err := jsonpatch.CreatePatch(req.Object.Raw, raw)
			if err != nil {
				return toAdmissionError(http.StatusInternalServerError, errors.Wrap(err, "error creating mutation patch"))
			}

			if len(patches) > 0 {
				return &admissionv1.AdmissionResponse{
					// todo: add Result
					PatchType: &[]admissionv1.PatchType{admissionv1.PatchTypeJSONPatch}[0],
					Patch:     jsonEncode(patches),
					Allowed:   true,
				}
			} else {
				return &admissionv1.AdmissionResponse{
					// todo: add Result
					Allowed: true,
				}
			}
		},
		log: log,
	}
}

// Register mutating webhook with router (such as http.ServeMux or gorilla's mux.Router).
// The type parameter T can be a pointer to a concrete Kubernetes resource type (such as *corev1.Pod),
// a pointer to unstructured.Unstructured, or an interface type containing runtime.Object;
// in the first case, scheme is required and must recognize the supplied resource type; in the second and third case,
// scheme is ignored (can be passed as nil), and a pointer to unstructured.Unstructured will be passed to
// the webhook implementation.
func RegisterMutatingWebhookWithRouter[T runtime.Object](w MutatingWebhook[T], scheme *runtime.Scheme, log logr.Logger, router Router) error {
	var obj T
	objType := reflect.TypeOf(obj)
	if objType == nil || objType.Kind() == reflect.Interface {
		log.Info("registering generic mutation webhook")

		path := "/generic/mutate"
		log.V(1).Info("starting handler", "path", path)
		router.Handle(path, NewMutatingWebhookHandler(w, scheme, log.WithValues("type", "generic mutation")))
	} else if objType.Kind() == reflect.Pointer {
		obj = reflect.New(objType.Elem()).Interface().(T)

		if _, ok := any(obj).(*unstructured.Unstructured); ok {
			log.Info("registering generic mutation webhook")

			path := "/generic/mutate"
			log.V(1).Info("starting handler", "path", path)
			router.Handle(path, NewMutatingWebhookHandler(w, scheme, log.WithValues("type", "generic mutation")))
		} else {
			log.Info("registering mutation webhook", "type", fmt.Sprintf("%T", obj))

			if scheme == nil {
				return fmt.Errorf("encountering empty/missing scheme")
			}
			gvks, unversioned, err := scheme.ObjectKinds(obj)
			if err != nil {
				return errors.Wrapf(err, "error fetching scheme information for type %T", obj)
			}
			if unversioned {
				return fmt.Errorf("encountering unversioned object type %T; unversioned types are not supported", obj)
			}

			for _, gvk := range gvks {
				if gvk.Group == "" {
					gvk.Group = "core"
				}
				path := "/" + strings.ToLower(gvk.Group) + "/" + strings.ToLower(gvk.Version) + "/" + strings.ToLower(gvk.Kind) + "/mutate"
				log.V(1).Info("starting handler", "path", path)
				router.Handle(path, NewMutatingWebhookHandler(w, scheme, log.WithValues("group", gvk.Group, "version", gvk.Version, "kind", gvk.Kind, "type", "mutation")))
			}
		}
	} else {
		return fmt.Errorf("encountering unsupported object kind %s", objType.Kind())
	}

	return nil
}

// Register mutating webhook to be served by Serve().
// Must be called before Serve().
// The type parameter T can be a pointer to a concrete Kubernetes resource type (such as *corev1.Pod),
// a pointer to unstructured.Unstructured, or an interface type containing runtime.Object;
// in the first case, scheme is required and must recognize the supplied resource type; in the second and third case,
// scheme is ignored (can be passed as nil), and a pointer to unstructured.Unstructured will be passed to
// the webhook implementation.
func RegisterMutatingWebhook[T runtime.Object](w MutatingWebhook[T], scheme *runtime.Scheme, log logr.Logger) error {
	return RegisterMutatingWebhookWithRouter(w, scheme, log, http.DefaultServeMux)
}

// Register a joint webhook (i.e. being validating and mutating at the same time) with router (such as http.ServeMux or gorilla's mux.Router).
// The type parameter T can be a pointer to a concrete Kubernetes resource type (such as *corev1.Pod),
// a pointer to unstructured.Unstructured, or an interface type containing runtime.Object;
// in the first case, scheme is required and must recognize the supplied resource type; in the second and third case,
// scheme is ignored (can be passed as nil), and a pointer to unstructured.Unstructured will be passed to
// the webhook implementation.
func RegisterWebhookWithRouter[T runtime.Object](w Webhook[T], scheme *runtime.Scheme, log logr.Logger, router Router) error {
	if err := RegisterValidatingWebhookWithRouter[T](w, scheme, log, router); err != nil {
		return err
	}
	if err := RegisterMutatingWebhookWithRouter[T](w, scheme, log, router); err != nil {
		return err
	}
	return nil
}

// Register a joint webhook (i.e. being validating and mutating at the same time) to be served by Serve().
// Must be called before Serve().
// The type parameter T can be a pointer to a concrete Kubernetes resource type (such as *corev1.Pod),
// a pointer to unstructured.Unstructured, or an interface type containing runtime.Object;
// in the first case, scheme is required and must recognize the supplied resource type; in the second and third case,
// scheme is ignored (can be passed as nil), and a pointer to unstructured.Unstructured will be passed to
// the webhook implementation.
func RegisterWebhook[T runtime.Object](w Webhook[T], scheme *runtime.Scheme, log logr.Logger) error {
	return RegisterWebhookWithRouter(w, scheme, log, http.DefaultServeMux)
}

// Options for webhook http server.
// Protocol https (and therefore CertFile and KeyFile) is mandatory
type ServeOptions struct {
	// Bind address, such as :2443 or 127.0.0.1:2443
	BindAddress string
	// Path to file containing the server TLS certificate (plus intermediates if present)
	CertFile string
	// PAth to file container the server TLS key
	KeyFile string
}

// Start webhook server.
// Parameter options may be nil; if it is nil then options will be taken from flags.
// Note that this requires that admission.InitFlags() and flag.Parse() (or equivalent) has been already called.
func Serve(ctx context.Context, options *ServeOptions) error {
	if options == nil {
		options = &optionsFromFlags
	}
	if options.BindAddress == "" {
		return fmt.Errorf("no bind address was specified")
	}
	if options.CertFile == "" {
		return fmt.Errorf("no TLS certificate file was specified")
	}
	if options.KeyFile == "" {
		return fmt.Errorf("no TLS key file was specified")
	}

	http.HandleFunc("/healthz", handleHealthz)

	server := &http.Server{Addr: options.BindAddress}
	ctxCh := ctx.Done()
	errCh := make(chan error)
	go func() {
		errCh <- server.ListenAndServeTLS(options.CertFile, options.KeyFile)
	}()
	for {
		select {
		case <-ctxCh:
			ctxCh = nil
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := server.Shutdown(ctx); err != nil {
				return err
			}
		case err := <-errCh:
			if err == http.ErrServerClosed {
				return nil
			}
			return err
		}
	}
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	// return empty content
}

func handleAdmission(w http.ResponseWriter, r *http.Request, admitFunc func(logr.Logger, context.Context, *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse, log logr.Logger) {
	var body []byte

	if r.Body == nil {
		err := fmt.Errorf("empty request")
		log.Error(err, "error handling admission request", "code", http.StatusBadRequest, "status", http.StatusText(http.StatusBadRequest))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if data, err := io.ReadAll(r.Body); err == nil {
		body = data
	} else {
		err := errors.Wrap(err, "error reading request body")
		log.Error(err, "error handling admission request", "code", http.StatusInternalServerError, "status", http.StatusText(http.StatusInternalServerError))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		err := fmt.Errorf("request has invalid content type %s; expected application/json", contentType)
		log.Error(err, "error handling admission request", "code", http.StatusUnsupportedMediaType, "status", http.StatusText(http.StatusUnsupportedMediaType))
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		return
	}

	log.V(4).Info("handling http request", "body", body)

	requestedAdmissionReview := admissionv1.AdmissionReview{}
	// todo: is the following really safe ?
	// what happens if an AdmissionReview v1beta1 object or something completely different is sent ?
	if _, _, err := decoder.Decode(body, nil, &requestedAdmissionReview); err != nil {
		err := errors.Wrap(err, "error deserializing admission review request")
		log.Error(err, "error handling admission request", "code", http.StatusBadRequest, "status", http.StatusText(http.StatusBadRequest))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.V(5).Info("admission request", "request", requestedAdmissionReview.Request)

	log = log.WithValues("operation", requestedAdmissionReview.Request.Operation, "namespace", requestedAdmissionReview.Request.Namespace, "name", requestedAdmissionReview.Request.Name)

	responseAdmissionReview := admissionv1.AdmissionReview{}
	responseAdmissionReview.APIVersion = requestedAdmissionReview.APIVersion
	responseAdmissionReview.Kind = requestedAdmissionReview.Kind
	responseAdmissionReview.Response = admitFunc(log, logr.NewContext(context.Background(), log), requestedAdmissionReview.Request)
	responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID

	log.V(5).Info("admission response", "response", responseAdmissionReview.Response)

	respBytes, err := json.Marshal(responseAdmissionReview)
	if err != nil {
		err := errors.Wrap(err, "error serializing admission review response")
		log.Error(err, "error handling admission request", "code", http.StatusInternalServerError, "status", http.StatusText(http.StatusInternalServerError))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(respBytes); err != nil {
		// not sure what else we could do here (this will result in a disconnect to the client)
		panic(err)
	}
}
