package localization

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// Sprintf 按语言包文本格式化字符串。
func Sprintf(key string, args ...any) string {
	return fmt.Sprintf(T(key), args...)
}

// SprintfDefault 按语言包文本格式化字符串，缺失时使用默认文本。
func SprintfDefault(key, fallback string, args ...any) string {
	return fmt.Sprintf(TextDefault(key, fallback), args...)
}

// Err 根据语言包文本创建错误。
func Err(key string) error {
	return errors.New(T(key))
}

// ErrDefault 根据语言包文本创建错误，缺失时使用默认文本。
func ErrDefault(key, fallback string) error {
	return errors.New(TextDefault(key, fallback))
}

// Errf 按语言包文本格式化错误。
func Errf(key string, args ...any) error {
	return fmt.Errorf(T(key), args...)
}

// ErrfDefault 按语言包文本格式化错误，缺失时使用默认文本。
func ErrfDefault(key, fallback string, args ...any) error {
	return fmt.Errorf(TextDefault(key, fallback), args...)
}

// Println 向标准输出打印文本。
func Println(args ...any) (int, error) {
	return fmt.Fprintln(os.Stdout, args...)
}

// Printf 按语言包文本格式化并打印到标准输出。
func Printf(key string, args ...any) (int, error) {
	return fmt.Fprintf(os.Stdout, T(key), args...)
}

// Fprintln 向指定输出打印文本。
func Fprintln(w io.Writer, args ...any) (int, error) {
	return fmt.Fprintln(w, args...)
}

// Fprintf 按语言包文本格式化并打印到指定输出。
func Fprintf(w io.Writer, key string, args ...any) (int, error) {
	return fmt.Fprintf(w, T(key), args...)
}
