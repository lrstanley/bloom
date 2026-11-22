// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The widths of the table's columns but the image's, which takes the rest.
const (
	serviceWidth = 10
	statusWidth  = 11
	secretWidth  = 14
)

// deployment is a row of the table. A status that needs explaining has a
// note, its tooltip.
type deployment struct {
	service string
	image   string
	status  string
	note    string
	secret  string
}

var deployments = []deployment{
	{"api", "registry.internal/platform/api:v2.14.3-rc.1+build.8812", "healthy", "", "sk_live_4eC39HqLyjWDarjtT1"},
	{"web", "registry.internal/web:v5.2.0", "healthy", "", "sk_live_9fA02LmQpZbXcVt7Rk"},
	{"payments", "registry.internal/payments/ledger-api:2026.11.3-hotfix.2", "degraded", "2 of 3 replicas ready · restarted 4m ago", "sk_live_T7yH1nBvC3xZqW8e"},
	{"search", "registry.internal/search:v1.9.4", "healthy", "", "sk_live_Mn5Kp2Qs8Xr4Tv1w"},
	{"worker", "registry.internal/jobs/worker-batch:v3.0.0-beta.7-arm64", "rolling", "rolling out · 4 of 6 pods updated", "sk_live_Hb6Gc9Jd2Ke5Lf8m"},
	{"mailer", "registry.internal/notify/mailer:v0.8.1-patch.3", "failed", "CrashLoopBackOff · exit code 137 (out of memory)", "sk_live_Qa3Wz7Ex1Rc5Tv9y"},
}

// table is the table of deployments, a row of which can be selected, with a
// click or the keyboard.
type table struct {
	st       styles
	selected int // the selected row, or -1
}

func (t *table) Layout(bloom.Context) bloom.Child {
	rows := []bloom.Child{tableRow(bloom.Text(""),
		bloom.Text("SERVICE").Style(t.st.header), bloom.Text("IMAGE").Style(t.st.header),
		bloom.Text("STATUS").Style(t.st.header), bloom.Text("SECRET").Style(t.st.header))}
	for i := range deployments {
		rows = append(rows, t.deploymentRow(i))
	}
	return bloom.Column(rows...)
}

// key moves the selection up and down.
func (t *table) key(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "up", "k":
		t.selected = max(t.selected-1, 0)
	case "down", "j":
		t.selected = min(t.selected+1, len(deployments)-1)
	}
}

// tableRow is a row of the table, with its cells in their columns, after
// the mark of the selected row.
func tableRow(mark, service, image, status, secret bloom.TextNode) bloom.FlexNode {
	return bloom.Row(
		bloom.Fixed(1, mark),
		bloom.Fixed(serviceWidth, service.Truncate()),
		bloom.Fill(image.Truncate()),
		bloom.Fixed(statusWidth, status),
		bloom.Fixed(secretWidth, secret.Truncate()),
	).Gap(1)
}

// deploymentRow is row i of the table. Its image and secret are cut to fit:
// the image unfolds under the pointer, and is pinned open while the row is
// selected, and the secret never shows in full.
func (t *table) deploymentRow(i int) bloom.Child {
	d := deployments[i]
	mark := bloom.Text("")
	if i == t.selected {
		mark = bloom.Text("›").Style(t.st.marker)
	}
	image := bloom.Text(d.image).Style(t.st.text).TooltipOpen(i == t.selected)
	secret := bloom.Text(d.secret).Style(t.st.label).NoTooltip()
	row := tableRow(mark, bloom.Text(d.service).Style(t.st.text), image, t.status(d), secret)
	return row.On(on.Click(func() tea.Cmd {
		t.selected = i
		return nil
	}))
}

// status is the status cell of d, with its note as a tooltip.
func (t *table) status(d deployment) bloom.TextNode {
	var st lipgloss.Style
	var icon string
	var opts []bloom.TipOption
	switch d.status {
	case "healthy":
		st, icon = t.st.healthy, "●"
	case "degraded":
		st, icon = t.st.degraded, "◐"
	case "rolling":
		st, icon = t.st.rolling, "◌"
	case "failed":
		st, icon = t.st.failed, "✕"
		opts = append(opts, bloom.TipStyle(t.st.failedTip))
	}
	return bloom.Text(icon+" "+d.status).Style(st).Tooltip(d.note, opts...)
}
