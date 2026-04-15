package config

type StartupWarning struct {
	Code       string `json:"code"`
	FilePath   string `json:"filePath"`
	BackupPath string `json:"backupPath,omitempty"`
	Message    string `json:"message"`
}
