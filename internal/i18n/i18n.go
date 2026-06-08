// Package i18n provides lightweight .po-based internationalization.
//
// po - globalization 的核心机制:
//   - 所有面向用户的字符串用 i18n.G() 包裹
//   - G() 根据 LANGUAGE/LC_ALL/LANG 环境变量选择语言
//   - 在 locales/ 目录下存放 .po 文件: locales/<lang>/slingshot.po
//   - 找不到翻译时回退到原文 (msgid)
//
// 这与 "在英文环境显示中文" 相反——它是让每个语言的用户看到自己的语言。
package i18n

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var (
	// 缓存已加载的翻译表: lang -> { msgid -> msgstr }
	translations map[string]map[string]string

	// 检测到的语言 (从环境变量)
	detectedLang string

	once sync.Once
)

// poLineRE 匹配 .po 文件中的 msgid/msgstr 行
var poLineRE = regexp.MustCompile(`^(msgid|msgstr)\s+"(.*)"\s*$`)

// detectLang 从环境变量检测用户语言。
// 优先级: LANGUAGE > LC_ALL > LANG
// 取第一个非空值, 并按 '_' 或 '.' 分割取语言代码前两部分 (如 "zh_CN").
func detectLang() string {
	for _, env := range []string{"LANGUAGE", "LC_ALL", "LANG"} {
		val := os.Getenv(env)
		if val == "" || val == "C" || val == "POSIX" {
			continue
		}

		// LANGUAGE 可以包含冒号分隔的列表
		lang := strings.Split(val, ":")[0]

		// 去掉 .UTF-8 等编码后缀
		if idx := strings.IndexByte(lang, '.'); idx >= 0 {
			lang = lang[:idx]
		}

		if lang != "" {
			return lang
		}
	}

	return "en_US"
}

// loadTranslations 扫描 locales/ 目录并加载所有 .po 文件。
func loadTranslations() {
	translations = make(map[string]map[string]string)
	detectedLang = detectLang()

	// 从可执行文件相对路径查找 locales
	exe, err := os.Executable()
	if err != nil {
		return
	}

	// 尝试多个可能的位置
	candidates := []string{
		filepath.Join(filepath.Dir(exe), "..", "..", "locales"), // 开发模式
		filepath.Join(filepath.Dir(exe), "locales"),             // 安装模式
		"locales", // 当前目录
	}

	for _, base := range candidates {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			lang := entry.Name()
			poFile := filepath.Join(base, lang, "slingshot.po")

			data, err := os.ReadFile(poFile)
			if err != nil {
				continue
			}

			table := parsePO(string(data))
			if len(table) > 0 {
				translations[lang] = table
			}
		}
	}
}

// parsePO 解析 .po 文件内容, 返回 msgid→msgstr 映射表。
func parsePO(data string) map[string]string {
	table := make(map[string]string)

	var currentID string
	var currentStr string
	var inID bool
	var inStr bool

	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()

		// 忽略注释和空行
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			// 保存上一对
			if currentID != "" {
				table[currentID] = currentStr
				currentID = ""
				currentStr = ""
			}
			inID = false
			inStr = false
			continue
		}

		// 处理 msgid 和 msgstr
		if matches := poLineRE.FindStringSubmatch(trimmed); len(matches) == 3 {
			key := matches[1]
			val := matches[2]

			// 处理 C 风格转义 (简化)
			val = strings.ReplaceAll(val, `\\`, `\`)
			val = strings.ReplaceAll(val, `\"`, `"`)
			val = strings.ReplaceAll(val, `\n`, "\n")

			if key == "msgid" {
				// 保存上一对
				if currentID != "" {
					table[currentID] = currentStr
				}
				currentID = val
				currentStr = ""
				inID = true
				inStr = false
			} else if key == "msgstr" {
				currentStr = val
				inID = false
				inStr = true
			}
		} else if inID {
			// 多行 msgid (通常不常见, 但支持)
			cont := strings.TrimSpace(line)
			cont = strings.Trim(cont, `"`)
			currentID += cont
		} else if inStr {
			// 多行 msgstr
			cont := strings.TrimSpace(line)
			cont = strings.Trim(cont, `"`)
			currentStr += cont
		}
	}

	// 保存最后一对
	if currentID != "" {
		table[currentID] = currentStr
	}

	return table
}

// G 返回 msgid 的翻译。
// 如果当前语言没有翻译, 返回 msgid 本身 (回退到英文)。
func G(msgid string) string {
	once.Do(loadTranslations)

	if table, ok := translations[detectedLang]; ok {
		if translated, ok := table[msgid]; ok && translated != "" {
			return translated
		}
	}

	// 尝试仅语言代码 (如 "zh" 从 "zh_CN")
	if idx := strings.IndexByte(detectedLang, '_'); idx >= 0 {
		shortLang := detectedLang[:idx]
		if table, ok := translations[shortLang]; ok {
			if translated, ok := table[msgid]; ok && translated != "" {
				return translated
			}
		}
	}

	return msgid
}

// SetLocale 强制设置语言 (用于测试)。
func SetLocale(lang string) {
	once.Do(loadTranslations)
	detectedLang = lang
}

// DumpTranslations 返回当前加载的翻译统计 (用于诊断)。
func DumpTranslations() string {
	once.Do(loadTranslations)
	var b strings.Builder
	fmt.Fprintf(&b, "detected lang: %s\n", detectedLang)
	for lang, table := range translations {
		fmt.Fprintf(&b, "  %s: %d entries\n", lang, len(table))
	}
	return b.String()
}
