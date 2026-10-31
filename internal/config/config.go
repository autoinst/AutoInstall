package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/autoinst/AutoInstall/internal/model"
)

var ErrMissingTarget = errors.New("未找到安装目标：请提供整合包、--mc-version 或 inst.json 中的 version")

type CLI struct {
	Target    model.Target
	Options   model.Options
	Modpack   string
	NoModpack bool
	Clean     bool
	Help      bool
	Version   bool
	set       map[string]bool
}

func Defaults() model.Options {
	return model.Options{Download: "official", MaxConnections: 4, MaxRetries: 3,
		FollowModpackFilenames: true, EnableModClassifier: true}
}

func flags(c *CLI, out io.Writer) *flag.FlagSet {
	f := flag.NewFlagSet("AutoInstall", flag.ContinueOnError)
	f.SetOutput(out)
	f.StringVar(&c.Target.Version, "mc-version", "", "Minecraft 版本（支持 latest）")
	f.StringVar(&c.Target.Loader, "loader", "", "vanilla / fabric / forge / neoforge")
	f.StringVar(&c.Target.LoaderVersion, "loader-version", "", "加载器版本（支持 latest）")
	f.StringVar(&c.Modpack, "modpack", "", "指定整合包或 metadata 文件，不扫描其他输入")
	f.BoolVar(&c.NoModpack, "no-modpack", false, "不自动发现整合包")
	f.StringVar(&c.Options.Download, "download", "official", "下载源：official / bmclapi（none 兼容 official）")
	f.IntVar(&c.Options.MaxConnections, "maxconnections", 4, "最大并发下载数")
	f.IntVar(&c.Options.MaxRetries, "maxretries", 3, "每个来源最多尝试次数")
	f.StringVar(&c.Options.Argsment, "argsment", "", "JVM 参数；显式空字符串清空配置值")
	f.BoolVar(&c.Options.FollowModpackFilenames, "follow-modpack-filenames", true, "使用整合包声明的模组文件名；关闭请用 =false")
	f.BoolVar(&c.Options.EnableModClassifier, "mod-classifier", true, "检测并移走客户端模组；关闭请用 =false")
	f.BoolVar(&c.Clean, "clean", false, "安装成功后清理本次拥有的临时内容")
	f.BoolVar(&c.Help, "help", false, "显示帮助")
	f.BoolVar(&c.Version, "version", false, "显示 AutoInstall 自身版本")
	return f
}

func Parse(args []string) (CLI, error) {
	var c CLI
	var output bytes.Buffer
	f := flags(&c, &output)
	if err := f.Parse(args); err != nil {
		return c, fmt.Errorf("参数错误: %s", strings.TrimSpace(output.String()))
	}
	c.set = make(map[string]bool)
	f.Visit(func(v *flag.Flag) { c.set[v.Name] = true })
	if c.Help || c.Version {
		return c, nil
	}
	if f.NArg() > 0 {
		return c, fmt.Errorf("不支持的位置参数: %s", strings.Join(f.Args(), " "))
	}
	if c.set["modpack"] && strings.TrimSpace(c.Modpack) == "" {
		return c, fmt.Errorf("--modpack 不能为空")
	}
	if c.NoModpack && c.set["modpack"] {
		return c, fmt.Errorf("--modpack 与 --no-modpack 不能同时使用")
	}
	for _, item := range []struct{ name, value string }{{"mc-version", c.Target.Version}, {"loader", c.Target.Loader}, {"loader-version", c.Target.LoaderVersion}} {
		if c.set[item.name] && strings.TrimSpace(item.value) == "" {
			return c, fmt.Errorf("--%s 不能为空", item.name)
		}
	}
	return c, nil
}

func Help(out io.Writer) {
	fmt.Fprintln(out, "AutoInstall — Minecraft 服务端安装器")
	fmt.Fprintln(out, "用法: AutoInstall [选项]\n无目标参数时自动发现整合包；显式目标参数进入普通安装。覆盖包目标需同时指定 --modpack。\ninst.json 可选，整合包安装不生成或改写它。")
	var c CLI
	flags(&c, out).PrintDefaults()
}

func (c CLI) DiscoverAutomatically() bool {
	return !c.NoModpack && !c.set["mc-version"] && !c.set["loader"] && !c.set["loader-version"]
}

func Load(path string, pack *model.Target, c CLI) (model.InstallRequest, []string, error) {
	r := model.InstallRequest{Options: Defaults()}
	var raw map[string]json.RawMessage
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return r, nil, fmt.Errorf("读取配置: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return r, nil, fmt.Errorf("解析配置: %w", err)
		}
		if raw == nil {
			return r, nil, fmt.Errorf("配置必须是 JSON 对象")
		}
	}
	fields := []struct {
		name  string
		value any
	}{
		{"maxconnections", &r.MaxConnections}, {"maxretries", &r.MaxRetries}, {"argsment", &r.Argsment},
		{"followModpackFilenames", &r.FollowModpackFilenames}, {"enableModClassifier", &r.EnableModClassifier},
	}
	if pack == nil {
		fields = append(fields, struct {
			name  string
			value any
		}{"version", &r.Version}, struct {
			name  string
			value any
		}{"loader", &r.Loader}, struct {
			name  string
			value any
		}{"loaderVersion", &r.LoaderVersion}, struct {
			name  string
			value any
		}{"download", &r.Download})
	} else {
		r.Target = *pack
	}
	for _, f := range fields {
		if b, ok := raw[f.name]; ok && string(b) != "null" {
			if err := json.Unmarshal(b, f.value); err != nil {
				return r, nil, fmt.Errorf("配置字段 %s: %w", f.name, err)
			}
		}
	}
	before := r.Target
	if c.set["loader"] && !strings.EqualFold(strings.TrimSpace(c.Target.Loader), r.Loader) && !c.set["loader-version"] && !strings.EqualFold(c.Target.Loader, "vanilla") {
		return r, nil, fmt.Errorf("切换加载器时需指定 --loader-version（可用 latest）")
	}
	if c.set["mc-version"] {
		r.Version = c.Target.Version
	}
	if c.set["loader"] {
		r.Loader = c.Target.Loader
	}
	if c.set["loader-version"] {
		r.LoaderVersion = c.Target.LoaderVersion
	}
	if c.set["download"] {
		r.Download = c.Options.Download
	}
	if c.set["maxconnections"] {
		r.MaxConnections = c.Options.MaxConnections
	}
	if c.set["maxretries"] {
		r.MaxRetries = c.Options.MaxRetries
	}
	if c.set["argsment"] {
		r.Argsment = c.Options.Argsment
	}
	if c.set["follow-modpack-filenames"] {
		r.FollowModpackFilenames = c.Options.FollowModpackFilenames
	}
	if c.set["mod-classifier"] {
		r.EnableModClassifier = c.Options.EnableModClassifier
	}
	r.Version = strings.TrimSpace(r.Version)
	r.Loader = strings.ToLower(strings.TrimSpace(r.Loader))
	r.LoaderVersion = strings.TrimSpace(r.LoaderVersion)
	r.Download = strings.ToLower(strings.TrimSpace(r.Download))
	if r.Loader == "" {
		r.Loader = "vanilla"
	}
	if r.Loader == "vanilla" {
		r.LoaderVersion = ""
	}
	if r.Download == "none" || r.Download == "" {
		r.Download = "official"
	}
	if r.Version == "" {
		return r, nil, ErrMissingTarget
	}
	switch r.Loader {
	case "vanilla", "fabric", "forge", "neoforge":
	default:
		return r, nil, fmt.Errorf("不支持的加载器: %s", r.Loader)
	}
	for _, value := range []string{r.Version, r.LoaderVersion} {
		if value == "." || value == ".." || strings.ContainsAny(value, "/\\\x00<>:\"|?*") {
			return r, nil, fmt.Errorf("版本字段不能包含路径或非法文件名字符: %q", value)
		}
		for _, char := range value {
			if char < 32 {
				return r, nil, fmt.Errorf("版本字段不能包含控制字符")
			}
		}
	}
	if r.Loader != "vanilla" && r.LoaderVersion == "" {
		return r, nil, fmt.Errorf("加载器 %s 缺少版本，请指定 --loader-version（可用 latest）", r.Loader)
	}
	if r.Download != "official" && r.Download != "bmclapi" {
		return r, nil, fmt.Errorf("不支持的下载源: %s", r.Download)
	}
	if r.MaxConnections <= 0 || r.MaxRetries <= 0 {
		return r, nil, fmt.Errorf("maxconnections 和 maxretries 必须为正整数")
	}
	var warnings []string
	if pack != nil && before != r.Target {
		warnings = append(warnings, fmt.Sprintf("整合包目标被显式覆盖：%+v -> %+v；仅改变服务端目标，不会转换或升级模组，兼容性需自行确认", before, r.Target))
	}
	if !r.EnableModClassifier {
		warnings = append(warnings, "已关闭模组分类器，不进行额外客户端检测或移动；不保证模组服务端兼容")
	}
	return r, warnings, nil
}
