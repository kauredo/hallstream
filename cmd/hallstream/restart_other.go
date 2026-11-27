//go:build !windows && !linux && !darwin

package main

import "github.com/kauredo/hallstream/internal/i18n"

func startCopy(string, []string) error { return i18n.Err("log.atualizacao.sem_programa") }
