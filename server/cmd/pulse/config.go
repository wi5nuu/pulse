package main

import (
	"net/url"

	"github.com/pulse/server/internal/config"
)

// loadConfig adalah thin wrapper di config.Load supaya main.go singkat.
func loadConfig() (config.Config, error) {
	return config.Load()
}

// redisAddr mengekstrak "host:port" dari redis URL.
// go-redis NewClient butuh Addr, bukan URL penuh. Parse URL, fallback ke
// "localhost:6379" jika parse gagal.
//
// Catatan: tidak menangani username:password di URL — Pulse Fase 1 (Redis lokal
// tanpa auth) tidak butuh. Tambah parsing auth di Fase 7 bila perlu.
func redisAddr(redisURL string) string {
	u, err := url.Parse(redisURL)
	if err != nil || u.Host == "" {
		return "localhost:6379"
	}
	return u.Host
}
