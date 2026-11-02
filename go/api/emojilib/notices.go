package emojilib

import "embed"

// Notices is embedded so deployed applications can expose the attribution and
// license texts even when the source checkout is unavailable.
//
//go:embed licenses/*
var Notices embed.FS
