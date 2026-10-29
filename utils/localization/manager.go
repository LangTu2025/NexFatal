package localization

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

const defaultLanguage = "zh"

//go:embed locales/*.properties
var embeddedLocales embed.FS

// Manager 管理内置和外部语言包。
type Manager struct {
	locales     map[string]map[string]string
	currentLang string
	localesDir  string
	mu          sync.RWMutex
}

var (
	globalManager *Manager
	managerOnce   sync.Once
)

// InitManager 初始化全局语言包管理器。
func InitManager(localesDir string) error {
	var err error
	managerOnce.Do(func() {
		globalManager = &Manager{
			locales:    make(map[string]map[string]string),
			localesDir: localesDir,
		}
		err = globalManager.init()
	})
	return err
}

// ReinitManager 重新创建全局语言包管理器。
func ReinitManager(localesDir string) error {
	globalManager = &Manager{
		locales:    make(map[string]map[string]string),
		localesDir: localesDir,
	}
	return globalManager.init()
}

// GetManager 返回全局语言包管理器。
func GetManager() *Manager {
	if globalManager == nil {
		_ = InitManager(filepath.Join(".", "locales"))
	}
	return globalManager
}

func (m *Manager) init() error {
	if err := m.loadAllLocales(); err != nil {
		return fmt.Errorf("failed to load locales: %w", err)
	}
	m.SetLanguage(defaultLanguage)
	return nil
}

func (m *Manager) loadAllLocales() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.locales = make(map[string]map[string]string)

	if err := m.loadEmbeddedLocales(); err != nil {
		return err
	}
	if err := m.loadExternalLocales(); err != nil {
		return err
	}

	return nil
}

func (m *Manager) loadEmbeddedLocales() error {
	entries, err := embeddedLocales.ReadDir("locales")
	if err != nil {
		return fmt.Errorf("failed to read embedded locales: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".properties") {
			continue
		}

		embeddedPath := path.Join("locales", entry.Name())
		data, err := embeddedLocales.ReadFile(embeddedPath)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", embeddedPath, err)
		}

		lang := strings.TrimSuffix(entry.Name(), ".properties")
		if err := m.loadLocale(lang, bytes.NewReader(data)); err != nil {
			return fmt.Errorf("failed to load embedded locale %s: %w", lang, err)
		}
	}

	return nil
}

func (m *Manager) loadExternalLocales() error {
	entries, err := os.ReadDir(m.localesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read locales directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".properties") {
			continue
		}

		lang := strings.TrimSuffix(entry.Name(), ".properties")
		filePath := filepath.Join(m.localesDir, entry.Name())
		if err := m.loadLocaleFile(lang, filePath); err != nil {
			return fmt.Errorf("failed to load locale %s: %w", lang, err)
		}
	}

	return nil
}

func (m *Manager) loadLocaleFile(lang, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open locale file: %w", err)
	}
	defer file.Close()
	return m.loadLocale(lang, file)
}

func (m *Manager) loadLocale(lang string, reader io.Reader) error {
	translations := make(map[string]string)
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\uFEFF"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}

		idx := strings.IndexAny(line, "=:")
		if idx <= 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key != "" {
			translations[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan locale file: %w", err)
	}

	m.locales[lang] = translations
	return nil
}

// SetLanguage 设置当前语言。
func (m *Manager) SetLanguage(lang string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentLang = strings.TrimSpace(lang)
}

// GetLanguage 返回当前语言。
func (m *Manager) GetLanguage() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentLang
}

// T 返回指定键对应的当前语言文本。
func (m *Manager) T(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if translations, ok := m.locales[m.currentLang]; ok {
		if text, ok := translations[key]; ok && strings.TrimSpace(text) != "" {
			return text
		}
	}

	if translations, ok := m.locales[defaultLanguage]; ok {
		if text, ok := translations[key]; ok && strings.TrimSpace(text) != "" {
			return text
		}
	}

	return key
}

// GetAvailableLanguages 返回已经加载的语言列表。
func (m *Manager) GetAvailableLanguages() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	langs := make([]string, 0, len(m.locales))
	for lang := range m.locales {
		langs = append(langs, lang)
	}
	return langs
}

// GetLanguageDisplayName 返回语言包显示名称。
func (m *Manager) GetLanguageDisplayName(lang string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if translations, ok := m.locales[lang]; ok {
		if displayName, ok := translations[KeyPackName]; ok && strings.TrimSpace(displayName) != "" {
			return displayName
		}
	}

	return lang
}

// GetAvailableLanguagesWithNames 返回语言代码到显示名称的映射。
func (m *Manager) GetAvailableLanguagesWithNames() map[string]string {
	result := make(map[string]string)
	for _, lang := range m.GetAvailableLanguages() {
		result[lang] = m.GetLanguageDisplayName(lang)
	}
	return result
}

// ReloadLocales 重新加载内置和外部语言包。
func (m *Manager) ReloadLocales() error {
	return m.loadAllLocales()
}

// T 返回全局管理器中的当前语言文本。
func T(key string) string {
	return GetManager().T(key)
}

// Text 是 T 的语义化别名。
func Text(key string) string {
	return T(key)
}

// TextDefault 返回语言文本，缺失时返回 fallback。
func TextDefault(key, fallback string) string {
	value := T(key)
	if value == key {
		return fallback
	}
	return value
}

// SetCurrentLanguage 设置全局当前语言。
func SetCurrentLanguage(lang string) {
	GetManager().SetLanguage(lang)
}

// GetCurrentLanguage 返回全局当前语言。
func GetCurrentLanguage() string {
	return GetManager().GetLanguage()
}

// GetAvailableLanguages 返回全局已加载语言列表。
func GetAvailableLanguages() []string {
	return GetManager().GetAvailableLanguages()
}

// GetLanguageDisplayName 返回指定语言的显示名称。
func GetLanguageDisplayName(lang string) string {
	return GetManager().GetLanguageDisplayName(lang)
}

// GetAvailableLanguagesWithNames 返回全局语言代码到显示名称的映射。
func GetAvailableLanguagesWithNames() map[string]string {
	return GetManager().GetAvailableLanguagesWithNames()
}
