// Package migrations meng-embed file SQL migration ke dalam binary server.
//
// Kenapa embed? Supaya deployment = satu binary. Tidak perlu copy folder
// migrations ke container/VM terpisah. Saat startup, server menjalankan
// goose.Up pada embed.FS ini untuk migrasi otomatis.
package migrations

import "embed"

// FS berisi semua file .sql di folder ini.
//
//go:embed *.sql
var FS embed.FS
