// Package i18n provides lightweight .po-based internationalization.
//
// po - globalization 的核心机制:
//   - 所有面向用户的字符串用 i18n.G() 包裹
//   - G() 根据 LANGUAGE/LC_ALL/LANG 环境变量选择语言
//   - 翻译文件在编译时通过 //go:embed 嵌入二进制 (locales/<lang>/slingshot.po)
//   - 找不到翻译时回退到原文 (msgid)
//
// 这与 "在英文环境显示中文" 相反——它是让每个语言的用户看到自己的语言。
package i18n

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	// 缓存已加载的翻译表: lang -> { msgid -> msgstr }
	// 通过 init() 预先加载所有嵌入的 .po 文件。
	translations map[string]map[string]string

	// 检测到的语言 (从环境变量)
	detectedLang string

	// 记录所有已加载的语言列表 (包括空表), 用于判断 .po 文件是否存在
	loadedLocales map[string]bool
)

// poLineRE 匹配 .po 文件中的 msgid/msgstr 行
var poLineRE = regexp.MustCompile(`^(msgid|msgstr)\s+"(.*)"\s*$`)

func init() {
	translations = make(map[string]map[string]string)
	loadedLocales = make(map[string]bool)
	loadAllTranslations()
	detectedLang = detectLang()
}

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
		lang, _, _ := strings.Cut(val, ":")

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

// loadAllTranslations 从嵌入的 locales/ 目录加载所有 .po 文件。
func loadAllTranslations() {
	entries, err := localesFS.ReadDir("locales")
	if err != nil {
		// 没有嵌入翻译数据 (编译时未找到匹配文件)
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		lang := entry.Name()
		data, err := localesFS.ReadFile("locales/" + lang + "/slingshot.po")
		if err != nil {
			continue
		}

		table := parsePO(string(data))
		translations[lang] = table
		loadedLocales[lang] = true
		if len(table) == 0 {
			// 空表也记录, 表示 .po 文件存在但无条目
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
			val = unescapePO(val)

			switch key {
			case "msgid":
				// 保存上一对
				if currentID != "" {
					table[currentID] = currentStr
				}
				currentID = val
				currentStr = ""
				inID = true
				inStr = false
			case "msgstr":
				currentStr = val
				inID = false
				inStr = true
			}
		} else if inID {
			// 多行 msgid (通常不常见, 但支持)
			cont := strings.TrimSpace(line)
			cont = strings.Trim(cont, `"`)
			cont = unescapePO(cont)
			currentID += cont
		} else if inStr {
			// 多行 msgstr
			cont := strings.TrimSpace(line)
			cont = strings.Trim(cont, `"`)
			cont = unescapePO(cont)
			currentStr += cont
		}
	}

	// 保存最后一对
	if currentID != "" {
		table[currentID] = currentStr
	}

	return table
}

// unescapePO 处理 PO 文件中的 C 风格转义。
func unescapePO(s string) string {
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\n`, "\n")
	return s
}

// G 返回 msgid 的翻译。
// 如果当前语言没有翻译, 返回 msgid 本身 (回退到英文)。
// 如果当前语言的 .po 文件存在但缺少该 msgid, 则 panic — 提醒开发者添加翻译条目。
func G(msgid string) string {
	if table, ok := translations[detectedLang]; ok {
		if translated, ok := table[msgid]; ok {
			if translated != "" {
				return translated
			}
			// found but empty string → 未翻译状态, 可容忍, 回退到 msgid
		} else {
			// key not in table → 开发者忘记添加 .po 条目
			panic(fmt.Sprintf("i18n: missing translation for %q in locale %s", msgid, detectedLang))
		}
	} else if loadedLocales[detectedLang] {
		// .po 文件存在但解析后为空表, 且缺少该 msgid → 忘记添加条目
		panic(fmt.Sprintf("i18n: missing translation for %q in locale %s (empty table)", msgid, detectedLang))
	}

	// 尝试仅语言代码 (如 "zh" 从 "zh_CN")
	if idx := strings.IndexByte(detectedLang, '_'); idx >= 0 {
		shortLang := detectedLang[:idx]
		if table, ok := translations[shortLang]; ok {
			if translated, ok := table[msgid]; ok {
				if translated != "" {
					return translated
				}
				// found but empty → 容忍, 继续回退
			}
			// 不 panic — 短代码表可能只是备选, 非主表
		}
	}

	return msgid
}

// SetLocale 强制设置语言 (用于测试)。
// 翻译表在 init() 中已全部加载, 切换 locale 无需重新加载。
func SetLocale(lang string) {
	detectedLang = lang
}

// CurrentLocale 返回当前语言设置。
func CurrentLocale() string {
	return detectedLang
}

// DumpTranslations 返回当前加载的翻译统计 (用于诊断)。
func DumpTranslations() string {
	var b strings.Builder
	fmt.Fprintf(&b, "detected lang: %s\n", detectedLang)
	for lang, table := range translations {
		fmt.Fprintf(&b, "  %s: %d entries\n", lang, len(table))
	}
	return b.String()
}
