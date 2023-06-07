/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and admission-webhook-runtime contributors
SPDX-License-Identifier: Apache-2.0
*/

package admission

import (
	"encoding/json"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func toAdmissionError(code int, err error) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Allowed: false,
		Result: &metav1.Status{
			Code:    int32(code),
			Reason:  metav1.StatusReason(http.StatusText(code)),
			Message: err.Error(),
		},
	}
}

func jsonEncode(obj any) []byte {
	raw, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return raw
}
