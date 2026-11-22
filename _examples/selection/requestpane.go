// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import "bloomfx.dev/bloom"

type field struct {
	name  string
	value string
}

var request = []field{
	{"id", "7f3a9c2e-41b8-4d0e-9a51-c2d8e6f01b77"},
	{"method", "POST"},
	{"path", "/api/v1/orders/1042/refund"},
	{"status", "502 Bad Gateway"},
	{"latency", "1.284s"},
	{"client", "203.0.113.42"},
	{"agent", "curl/8.11.0"},
	{"trace", "00-4bf92f3577b34da6a3ce929d0e0e4736-01"},
}

// failure is the error the request failed with, long enough to wrap.
const failure = "payments did not answer within 1000ms while refunding order 1042 " +
	"(attempt 1 of 3); the refund will be retried in 2s with the same idempotency key rfnd_7Hc2Lq9xVb"

// requestPane is the request, as one selectable column of fields, a value
// beside its label, and the error it failed with under them.
type requestPane struct{ st styles }

func (r *requestPane) Layout(bloom.Context) bloom.Child {
	rows := make([]bloom.Child, 0, len(request))
	for _, f := range request {
		rows = append(rows, bloom.Row(
			bloom.Fixed(9, bloom.Text(f.name).Style(r.st.label)),
			// Cut to fit, but copied whole.
			bloom.Fill(bloom.Text(f.value).Style(r.st.value).Truncate()),
		))
	}
	rows = append(rows,
		bloom.Fixed(1, nil),
		bloom.Text("error").Style(r.st.label),
		// Wrapped to the pane's width, but copied as the one line it is.
		bloom.Fill(bloom.Text(failure).Style(r.st.value)),
	)
	// The fields are selectable together, and the pane's border is not.
	return r.st.pane("request", bloom.Column(rows...).ID("request").Selectable())
}
