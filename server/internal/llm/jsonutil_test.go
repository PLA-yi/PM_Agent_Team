package llm

import "testing"

func TestExtractJSONObject(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"clean object", `{"a":1}`, `{"a":1}`},
		{"clean array", `[1,2,3]`, `[1,2,3]`},
		{"with prelude", `以下是结果：{"a":1}`, `{"a":1}`},
		{"with trailing", `{"a":1} 以上是 stories`, `{"a":1}`},
		{"both ends", `Sure! {"a":1} hope this helps`, `{"a":1}`},
		{"json fence", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"plain fence", "```\n{\"a\":1}\n```", `{"a":1}`},
		{"nested", `{"a":{"b":[1,2]},"c":"}}"}`, `{"a":{"b":[1,2]},"c":"}}"}`},
		{"string with brace", `{"a":"hi {x}"}`, `{"a":"hi {x}"}`},
		{"escaped quote", `{"a":"he said \"hi\""}`, `{"a":"he said \"hi\""}`},
		{"empty", ``, ``},
		{"no json", `just text`, `just text`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExtractJSONObject(c.in)
			if got != c.want {
				t.Errorf("\n  in:   %q\n  want: %q\n  got:  %q", c.in, c.want, got)
			}
		})
	}
}
