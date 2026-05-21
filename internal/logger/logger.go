package logger

import (
	"fmt"
	"os"
	"time"
)

// Info logs an informational message with the standard camply prefix
func Info(format string, a ...interface{}) {
	prefix := fmt.Sprintf("[%s] INFO     ", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("%s%s\n", prefix, fmt.Sprintf(format, a...))
}

// Error logs an error message
func Error(format string, a ...interface{}) {
	prefix := fmt.Sprintf("[%s] ERROR    ", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(os.Stderr, "%s%s\n", prefix, fmt.Sprintf(format, a...))
}

// Camply logs a generic camply message (like startup/exit)
func Camply(format string, a ...interface{}) {
	prefix := fmt.Sprintf("[%s] CAMPLY   ", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("%s%s\n", prefix, fmt.Sprintf(format, a...))
}
