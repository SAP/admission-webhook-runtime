# Kubernetes Admission Webhook Runtime

[![REUSE status](https://api.reuse.software/badge/github.com/SAP/admission-webhook-runtime)](https://api.reuse.software/info/github.com/SAP/admission-webhook-runtime)

## About this project

This repository contains a generic Kubernetes admission webhook implementation framework.

Consumers implement one or all of the following parameterized interfaces:

```go
// Validating webhook interface.
type ValidatingWebhook[T runtime.Object] interface {
	ValidateCreate(ctx context.Context, obj T) error
	ValidateUpdate(ctx context.Context, oldObj T, newObj T) error
	ValidateDelete(ctx context.Context, obj T) error
}

// Mutating webhook interface.
type MutatingWebhook[T runtime.Object] interface {
	MutateCreate(ctx context.Context, obj T) error
	MutateUpdate(ctx context.Context, oldObj T, newObj T) error
}
```

Two sorts of webhooks are supported:

- **Specific (typed) webhooks**

  A specific webhook is one which handles exactly one resource type (such as `corev1.Pod`).
  Using go's new generics concept allows the implementation of such webhooks to be strongly typed, such as:

  ```go
  type PodWebhook struct{}

  func (w *PodWebhook) MutateCreate(ctx context.Context, pod *corev1.Pod) error {
      // do the mutation (creation case); just modify the passed pod object ...
      // returning errors will result in rejection of the api request
      return nil
  }

  func (w *PodWebhook) MutateUpdate(ctx context.Context, oldPod *corev1.Pod, newPod *corev1.Pod) error {
      // do the mutation (update case); just modify the passed pod object ...
      // returning errors will result in rejection of the api request
      return nil
  }
  ```

  (notice the usage of `*corev1.Pod` in the method signatures).

  The according webhooks can then be reached at `/<group>/<version>/<kind>/validate` and `/<group>/<version>/<kind>/mutate`, respectively (where the parts are all lower case).

  A minimal but working implementation can be found [here](./examples/pod-mutation/main.go).

  Registrations (that is `ValidatingWebhookConfiguration`, `MutatingWebhookConfiguration` objects) for such typed webhooks must list exactly one API version in their matching rules
  (fitting the go type which the webhook defined for; in the above example: `corev1.Pod`). Such as:

  ```yaml
  ---
  apiVersion: admissionregistration.k8s.io/v1
  kind: MutatingWebhookConfiguration
  metadata:
    name: pod-admission
  webhooks:
  - admissionReviewVersions:
    # ...
    name: pod-mutation.cs.sap.com
    rules:
    - apiGroups:
      - ""
      apiVersions:
      - v1
      operations:
      - CREATE
      - UPDATE
      resources:
      - pods
    sideEffects: None
  ```

- **Generic (untyped) webhooks**

  Other than a typed webhook, a generic webhook can handle multiple resource types.
  This is achieved by either using an interface including `runtime.Object`, or `*unstructured.Unstructured` as a type parameter, such as:

  ```go
  type GenericWebhook struct{}

  func (w *GenericWebhook) MutateCreate(ctx context.Context, obj.runtime.Object) error {
      // do the mutation (creation case); just modify the passed object ...
      // returning errors will result in rejection of the api request
      return nil
  }

  func (w *GenericWebhook) MutateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) error {
      // do the mutation (update case); just modify the passed object ...
      // returning errors will result in rejection of the api request
      return nil
  }
  ```

  The according webhooks can then be reached at `/generic/validate` and `/generic/mutate`, respectively. Note that this obviously implies that there cannot be more than one generic validating and one mutating webhook registered.

  A minimal but working implementation can be found [here](./examples/generic-validation/main.go).

  A registration of such a webhook could look like this:

  ```yaml
  ---
  apiVersion: admissionregistration.k8s.io/v1
  kind: ValidatingWebhookConfiguration
  metadata:
    name: generic-admission
  webhooks:
  - admissionReviewVersions:
    # ...
    name: generic-validation.cs.sap.com
    rules:
    - apiGroups:
      - "*"
      apiVersions:
      - "*"
      operations:
      - CREATE
      - UPDATE
      resources:
      - "*"
    sideEffects: None
  ```

## Documentation

The API reference is here: [https://pkg.go.dev/github.com/sap/admission-webhook-runtime](https://pkg.go.dev/github.com/sap/admission-webhook-runtime).

## Support, Feedback, Contributing

This project is open to feature requests/suggestions, bug reports etc. via [GitHub issues](https://github.com/SAP/admission-webhook-runtime/issues). Contribution and feedback are encouraged and always welcome. For more information about how to contribute, the project structure, as well as additional contribution information, see our [Contribution Guidelines](CONTRIBUTING.md).

## Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/SAP/.github/blob/main/CODE_OF_CONDUCT.md) at all times.

## Licensing

Copyright 20xx SAP SE or an SAP affiliate company and admission-webhook-runtime contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/SAP/admission-webhook-runtime).
