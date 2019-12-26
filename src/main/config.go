package main

import (
	"sync"

	"github.com/ini"

)

type UpdateCfg struct {
	author          string
	exe_version     string
	source_dir      string
	source_exe_name string
	target_dir      string
	file_suffix     string
	server_type     string
	server_prefix   string
	backup_file_num int
	mu              sync.RWMutex
}

func NewUpdateCfg() *UpdateCfg {
	return &UpdateCfg{}
}

func (upcfg *UpdateCfg) Load(path string) error {
	cfg, err := ini.Load(path)
	if err != nil {
		return err
	}

	upcfg.mu.Lock()
	defer upcfg.mu.Unlock()

	upcfg.author = ""
	upcfg.exe_version = ""
	if sec, er := cfg.GetSection("Signature"); er == nil {
		if sec.HasKey("author") {
			upcfg.author = sec.Key("author").String()
			upcfg.exe_version = sec.Key("exe_version").String()
		}
	}

	upcfg.source_dir = ""
	upcfg.source_exe_name = ""
	upcfg.target_dir = ""
	upcfg.file_suffix = ""
	upcfg.server_type = ""
	upcfg.server_prefix = ""
	upcfg.backup_file_num = 3
	if sec, er := cfg.GetSection("Update_Cfg"); er == nil {
		if sec.HasKey("source_dir") {
			upcfg.source_dir = sec.Key("source_dir").String()
		}
		if sec.HasKey("source_exe_name") {
			upcfg.source_exe_name = sec.Key("source_exe_name").String()
		}
		if sec.HasKey("target_dir") {
			upcfg.target_dir = sec.Key("target_dir").String()
		}
		if sec.HasKey("file_suffix") {
			upcfg.file_suffix = sec.Key("file_suffix").String()
		}
		if sec.HasKey("server_type") {
			upcfg.server_type = sec.Key("server_type").String()
		}
		if sec.HasKey("server_prefix") {
			upcfg.server_prefix = sec.Key("server_prefix").String()
		}
		if sec.HasKey("backup_file_num") {
			upcfg.backup_file_num, _ = sec.Key("backup_file_num").Int()
		}
	}

	return nil
}
