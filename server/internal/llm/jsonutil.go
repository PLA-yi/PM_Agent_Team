package llm

import "strings"

// stripJSONFence 剥掉 ```json ... ``` 或 ``` ... ``` 包裹
func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.IndexByte(s, '\n'); i > 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}

// ExtractJSONObject 从字符串中提取第一个完整的 JSON 对象 / 数组。
// 处理常见 LLM 漏字：
//   - JSON 前后带自然语言（"以下是结果：{...}"）
//   - 带 ```json``` 围栏
//   - JSON 后追加说明文字（"以上是 stories"）
//
// 返回提取出的 JSON 字符串；若找不到则原样返回。
func ExtractJSONObject(s string) string {
	s = stripJSONFence(s)
	if s == "" {
		return s
	}
	// 找第一个 { 或 [
	start := -1
	open := byte(0)
	close := byte(0)
	for i := 0; i < len(s); i++ {
		if s[i] == '{' {
			start = i
			open = '{'
			close = '}'
			break
		}
		if s[i] == '[' {
			start = i
			open = '['
			close = ']'
			break
		}
	}
	if start < 0 {
		return s
	}
	// 平衡括号扫描，跳过字符串内的 { } [ ]
	depth := 0
	inStr := false
	escape := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			continue
		}
		if c == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		switch c {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	// 不平衡：原样返回让上层报错
	return s
}
