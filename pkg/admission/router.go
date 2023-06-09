/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and admission-webhook-runtime contributors
SPDX-License-Identifier: Apache-2.0
*/

package admission

import "net/http"

type Router interface {
	Handle(pattern string, handler http.Handler)
}
