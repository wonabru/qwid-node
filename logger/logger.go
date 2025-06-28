package logger

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var DefaultLogsHomePath = "/.okura/logs/"

var (
	logFile *os.File
	mw      *MultiWriter
	logger  *log.Logger
	once    sync.Once
)

type MultiWriter struct {
	writers []io.Writer
}

func (t *MultiWriter) Write(p []byte) (n int, err error) {
	for _, w := range t.writers {
		n, err = w.Write(p)
		if err != nil {
			return
		}
		if n != len(p) {
			err = io.ErrShortWrite
			return
		}
	}
	return len(p), nil
}

func InitLogger() {
	once.Do(func() {
		homePath, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		logsDir := filepath.Join(homePath, DefaultLogsHomePath)

		if err := os.MkdirAll(logsDir, 0755); err != nil {
			log.Fatal(err)
		}
		// Create daily log file
		timestamp := time.Now().Format("2006-01-02")
		logFilePath := filepath.Join(logsDir, "mining-"+timestamp+".log")
		logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal(err)
		}
		mw = &MultiWriter{
			writers: []io.Writer{
				os.Stdout,
				logFile,
			},
		}
		logger = log.New(mw, "", log.LstdFlags)
		log.SetOutput(mw)
		log.SetFlags(log.LstdFlags)
		// Start cleanup routine
		go cleanupOldLogs(logsDir)
	})
}

func GetLogger() *log.Logger {
	InitLogger()
	return logger
}

func CloseLogger() {
	if logFile != nil {
		logFile.Close()
	}
}

// cleanupOldLogs removes log files older than 7 days
func cleanupOldLogs(logsDir string) {
	for {
		time.Sleep(24 * time.Hour) // Run cleanup once per day
		rotateLogFile(logsDir)
		// Get all log files
		files, err := filepath.Glob(filepath.Join(logsDir, "mining-*.log"))
		if err != nil {
			log.Printf("Error getting log files: %v", err)
			continue
		}
		// Remove files older than 7 days
		for _, file := range files {
			info, err := os.Stat(file)
			if err != nil {
				log.Printf("Error getting file info: %v", err)
				continue
			}
			if time.Since(info.ModTime()) > 7*24*time.Hour {
				if err := os.Remove(file); err != nil {
					log.Printf("Error removing old log file: %v", err)
				}
			}
		}
	}
}

// rotateLogFile creates a new log file for the current day
func rotateLogFile(logsDir string) {
	if logFile != nil {
		logFile.Close()
	}
	var err error
	timestamp := time.Now().Format("2006-01-02")
	logFilePath := filepath.Join(logsDir, "mining-"+timestamp+".log")
	logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	mw.writers[1] = logFile
}
