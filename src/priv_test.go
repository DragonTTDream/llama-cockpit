package main

import "testing"

func TestValidateEnvBody(t *testing.T) {
	ok := []string{"", "CTX=8192\nNGL=999\n", "OPT=--temp 0.7 --jinja\n"}
	for _, s := range ok {
		if err := validateEnvBody(s); err != nil {
			t.Errorf("应通过但被拒: %q -> %v", s, err)
		}
	}
	bad := []string{
		"rm -rf /\n",                  // 没有 =
		"lower=1\n",                   // 小写键名
		"CTX=$(curl evil)\nnotakey\n", // 混入非 KEY=value
		"1BAD=1\n",                    // 数字开头
	}
	for _, s := range bad {
		if err := validateEnvBody(s); err == nil {
			t.Errorf("应被拒但通过了: %q", s)
		}
	}
}

func TestValidateUnitBody(t *testing.T) {
	good := `[Unit]
Description=x
[Service]
ExecStart=/bin/sh -c 'exec /usr/local/bin/llama-server --model "${MODEL}" $$OPT'
[Install]
WantedBy=multi-user.target
`
	if err := validateUnitBody(good); err != nil {
		t.Errorf("合法 unit 被拒: %v", err)
	}
	bad := map[string]string{
		"非 llama-server": "[Unit]\n[Service]\nExecStart=/bin/rm -rf /\n[Install]\nWantedBy=x\n",
		"缺段":             "[Service]\nExecStart=/usr/local/bin/llama-server ${MODEL} $$OPT\n",
		"无模板变量":          "[Unit]\n[Service]\nExecStart=/usr/local/bin/llama-server --evil\n[Install]\nWantedBy=x\n",
		"不以[Unit]开头":     "ExecStart=/usr/local/bin/llama-server ${MODEL} $$OPT\n[Service]\n[Install]\n",
	}
	for name, s := range bad {
		if err := validateUnitBody(s); err == nil {
			t.Errorf("应被拒但通过了: %s", name)
		}
	}
}

func TestUnitNamePattern(t *testing.T) {
	ok := []string{"llama-general", "llama-x", "llama-rerank", "llama-a1"}
	bad := []string{"llama-", "llama-A", "llama-x;rm -rf /", "../etc/passwd",
		"llama-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "other-service"}
	for _, n := range ok {
		if !unitNamePat.MatchString(n) {
			t.Errorf("应通过: %s", n)
		}
	}
	for _, n := range bad {
		if unitNamePat.MatchString(n) {
			t.Errorf("应被拒: %s", n)
		}
	}
}
