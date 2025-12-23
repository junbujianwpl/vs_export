package sln

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Project struct {
	ProjectDir          string
	ProjectPath         string
	XMlName             xml.Name              `xml:"Project"`
	PropertyGroup       []PropertyGroup       `xml:"PropertyGroup"`
	Import              []Import              `xml:"Import"`
	ItemGroup           []ItemGroup           `xml:"ItemGroup"`
	ItemDefinitionGroup []ItemDefinitionGroup `xml:"ItemDefinitionGroup"`
}

// 通用的ClCompile元素结构
type ClCompile struct {
	XMLName                      xml.Name `xml:"ClCompile"`
	Include                      string   `xml:"Include,attr"`
	AdditionalIncludeDirectories string   `xml:"AdditionalIncludeDirectories"`
	PreprocessorDefinitions      string   `xml:"PreprocessorDefinitions"`
	AdditionalOptions            string   `xml:"AdditionalOptions"`
}

type ItemGroup struct {
	XMLName                  xml.Name               `xml:"ItemGroup"`
	Label                    string                 `xml:"Label,attr"`
	ProjectConfigurationList []ProjectConfiguration `xml:"ProjectConfiguration"`
	// 合并两个字段为一个通用的ClCompile列表
	ClCompileList []ClCompile `xml:"ClCompile"`
}

type ProjectConfiguration struct {
	XMLName       xml.Name `xml:"ProjectConfiguration"`
	Include       string   `xml:"Include,attr"`
	Configuration string   `xml:"Configuration"`
	Platform      string   `xml:"Platform"`
}

type ItemDefinitionGroup struct {
	XMLName   xml.Name     `xml:"ItemDefinitionGroup"`
	Condition string       `xml:"Condition,attr"`
	ClCompile ClCompileDef `xml:"ClCompile"`
}

// ItemDefinitionGroup中的ClCompile元素
type ClCompileDef struct {
	XMLName                      xml.Name `xml:"ClCompile"`
	AdditionalIncludeDirectories string   `xml:"AdditionalIncludeDirectories"`
	PreprocessorDefinitions      string   `xml:"PreprocessorDefinitions"`
	// 添加更多编译选项支持
	AdditionalOptions string `xml:"AdditionalOptions"`
	WarningLevel      string `xml:"WarningLevel"`
	Optimization      string `xml:"Optimization"`
	RuntimeLibrary    string `xml:"RuntimeLibrary"`
	LanguageStandard  string `xml:"LanguageStandard"`
	// 支持Conan等包管理器的include路径
	AdditionalUsingDirectories string `xml:"AdditionalUsingDirectories"`
}

// 移除重复的ClCompileSrc结构体，使用统一的ClCompile结构

type CompileCommand struct {
	Dir  string `json:"directory"`
	Cmd  string `json:"command"`
	File string `json:"file"`
}

var (
	badInclude = []string{
		";%(AdditionalIncludeDirectories)",
		"%(AdditionalIncludeDirectories);",
	}
	badDef = []string{
		";%(PreprocessorDefinitions)",
		"%(PreprocessorDefinitions);",
	}
	badOpts = []string{"%(AdditionalOptions)"}
)

func NewProject(path string) (Project, error) {
	var pro Project
	var err error

	pro.ProjectPath, err = filepath.Abs(path)
	if err != nil {
		return pro, err
	}
	pro.ProjectDir = filepath.Dir(pro.ProjectPath)

	f, err := os.Open(path)
	if err != nil {
		return Project{}, err
	}
	defer f.Close()
	data, err := ioutil.ReadAll(f)
	err = xml.Unmarshal([]byte(data), &pro)
	if err != nil {
		return pro, err
	}
	return pro, nil
}

// 增强的配置查找函数，返回更完整的编译信息
func (pro *Project) FindConfigEnhanced(conf string) (string, string, string, string, error) {
	var cfgList []ProjectConfiguration
	for _, v := range pro.ItemGroup {
		if len(v.ProjectConfigurationList) > 0 {
			cfgList = v.ProjectConfigurationList
			break
		}
	}

	// 收集所有可用配置
	var availableConfigs []string
	for _, v := range cfgList {
		availableConfigs = append(availableConfigs, v.Include)
	}

	// 查找完全匹配的配置
	found := false
	matchedConfig := conf
	for _, v := range cfgList {
		if v.Include == conf {
			matchedConfig = v.Include
			found = true
			break
		}
	}

	// 如果完全匹配失败，尝试查找相同平台的其他配置
	if !found {
		// 解析用户请求的配置和平台
		requestedParts := strings.Split(conf, "|")
		if len(requestedParts) == 2 {
			requestedPlatform := requestedParts[1]

			// 查找相同平台的配置
			for _, v := range cfgList {
				configParts := strings.Split(v.Include, "|")
				if len(configParts) == 2 && configParts[1] == requestedPlatform {
					matchedConfig = v.Include
					found = true
					fmt.Fprintf(os.Stderr, "Warning: Configuration %s not found, using %s instead\n", conf, matchedConfig)
					break
				}
			}
		}
	}

	// 如果仍然没有找到匹配的配置，返回错误并列出可用配置
	if !found {
		return "", "", "", "", fmt.Errorf("%s:not found %s\nAvailable configurations: %v", pro.ProjectPath, conf, availableConfigs)
	}

	// 解析配置和平台
	vlist := strings.Split(matchedConfig, "|")
	configuration := vlist[0]
	platform := vlist[1]

	// 构建环境变量替换映射
	willReplaceEnv := map[string]string{
		"$(ProjectDir)":        pro.ProjectDir,
		"$(Configuration)":     configuration,
		"$(ConfigurationName)": configuration,
		"$(Platform)":          platform,
	}
	for _, v := range os.Environ() {
		kv := strings.SplitN(v, "=", 2)
		if len(kv) == 2 {
			willReplaceEnv[fmt.Sprintf("$(%s)", kv[0])] = kv[1]
		}
	}

	// 从PropertyGroup中收集include目录
	propertyIncludeDirs := []string{}
	for _, v := range pro.PropertyGroup {
		// 匹配条件
		if v.Condition == "" || strings.Contains(v.Condition, matchedConfig) {
			if v.AdditionalIncludeDirectories != "" {
				propertyIncludeDirs = append(propertyIncludeDirs, v.AdditionalIncludeDirectories)
			}
			if v.IncludeDirectories != "" {
				propertyIncludeDirs = append(propertyIncludeDirs, v.IncludeDirectories)
			}
		}
	}

	// 从ItemDefinitionGroup中收集配置
	var include string
	var def string
	var additionalOpts string
	var usingDirs string

	for _, v := range pro.ItemDefinitionGroup {
		// 使用匹配的配置而不是原始请求的配置
		if strings.Contains(v.Condition, matchedConfig) {
			cl := v.ClCompile
			include = cl.AdditionalIncludeDirectories
			def = cl.PreprocessorDefinitions
			additionalOpts = cl.AdditionalOptions
			usingDirs = cl.AdditionalUsingDirectories
			break
		}
	}

	// 合并PropertyGroup和ItemDefinitionGroup中的include目录
	if len(propertyIncludeDirs) > 0 {
		if include != "" {
			propertyIncludeDirs = append(propertyIncludeDirs, include)
			include = MergeSemicolonSeparatedLists(propertyIncludeDirs...)
		} else {
			include = MergeSemicolonSeparatedLists(propertyIncludeDirs...)
		}
	}

	// 处理所有字段的环境变量替换
	for k, v := range willReplaceEnv {
		if strings.Contains(include, k) {
			include = strings.Replace(include, k, v, -1)
		}
		if strings.Contains(def, k) {
			def = strings.Replace(def, k, v, -1)
		}
		if strings.Contains(additionalOpts, k) {
			additionalOpts = strings.Replace(additionalOpts, k, v, -1)
		}
		if strings.Contains(usingDirs, k) {
			usingDirs = strings.Replace(usingDirs, k, v, -1)
		}
	}

	return include, def, additionalOpts, usingDirs, nil
}

// return include, definition,error
func (pro *Project) FindConfig(conf string) (string, string, error) {
	var cfgList []ProjectConfiguration
	for _, v := range pro.ItemGroup {
		if len(v.ProjectConfigurationList) > 0 {
			cfgList = v.ProjectConfigurationList
			break
		}
	}

	// 收集所有可用配置
	availableConfigs := []string{}
	for _, v := range cfgList {
		availableConfigs = append(availableConfigs, v.Include)
	}

	if len(cfgList) == 0 {
		return "", "", fmt.Errorf("%s: no configurations found", pro.ProjectPath)
	}

	// 检查配置是否存在
	found := false
	var matchedConfig string

	// 首先尝试完全匹配
	for _, v := range cfgList {
		if v.Include == conf {
			found = true
			matchedConfig = conf
			break
		}
	}

	// 如果完全匹配失败，尝试查找相同平台的其他配置
	if !found {
		// 解析用户请求的配置和平台
		requestedParts := strings.Split(conf, "|")
		if len(requestedParts) == 2 {
			requestedPlatform := requestedParts[1]

			// 查找相同平台的配置
			for _, v := range cfgList {
				configParts := strings.Split(v.Include, "|")
				if len(configParts) == 2 && configParts[1] == requestedPlatform {
					matchedConfig = v.Include
					found = true
					fmt.Fprintf(os.Stderr, "Warning: Configuration %s not found, using %s instead\n", conf, matchedConfig)
					break
				}
			}
		}
	}

	// 如果仍然没有找到匹配的配置，返回错误并列出可用配置
	if !found {
		return "", "", fmt.Errorf("%s:not found %s\nAvailable configurations: %v", pro.ProjectPath, conf, availableConfigs)
	}
	for _, v := range pro.ItemDefinitionGroup {
		// 使用匹配的配置而不是原始请求的配置
		if strings.Contains(v.Condition, matchedConfig) {
			cl := v.ClCompile

			vlist := strings.Split(matchedConfig, "|")
			configuration := vlist[0]
			platform := vlist[1]

			willReplaceEnv := map[string]string{
				"$(ProjectDir)":        pro.ProjectDir,
				"$(Configuration)":     configuration,
				"$(ConfigurationName)": configuration,
				"$(Platform)":          platform,
			}
			for _, v := range os.Environ() {
				kv := strings.Split(v, "=")
				willReplaceEnv[fmt.Sprintf("$(%s)", kv[0])] = kv[1]
			}

			include := cl.AdditionalIncludeDirectories
			def := cl.PreprocessorDefinitions

			// 处理预处理器定义的环境变量替换
			for k, v := range willReplaceEnv {
				if strings.Contains(include, k) {
					include = strings.Replace(include, k, v, -1)
				}
				if strings.Contains(def, k) {
					def = strings.Replace(def, k, v, -1)
				}
			}

			re := regexp.MustCompile(`\$\(.+\)`)
			badEnv := re.FindAllString(include, -1)
			if len(badEnv) > 0 {
				//fmt.Fprintf(os.Stderr, "%s:bad env[%v]\n", pro.ProjectPath, badEnv[:])
				//for _, v := range badEnv {
				//	include = strings.Replace(include, v, "", -1)
				//}
			}

			return include, def, nil
		}
	}
	return "", "", errors.New("not found " + conf)
}

func (pro *Project) FindSourceFiles() []string {
	var fileList []string
	for _, v := range pro.ItemGroup {
		for _, clCompile := range v.ClCompileList {
			fileList = append(fileList, clCompile.Include)
		}
	}
	return fileList
}

// 收集ItemGroup中的额外配置
func (pro *Project) FindItemGroupConfigs(conf string) (string, string, string) {
	var extraIncludes, extraDefs, extraOpts []string

	// 遍历所有ItemGroup
	for _, itemGroup := range pro.ItemGroup {
		// 遍历当前ItemGroup中的所有ClCompile项
		for _, clCompile := range itemGroup.ClCompileList {
			// 收集所有ClCompile项的配置
			if strings.TrimSpace(clCompile.AdditionalIncludeDirectories) != "" {
				extraIncludes = append(extraIncludes, clCompile.AdditionalIncludeDirectories)
			}
			if strings.TrimSpace(clCompile.PreprocessorDefinitions) != "" {
				extraDefs = append(extraDefs, clCompile.PreprocessorDefinitions)
			}
			if strings.TrimSpace(clCompile.AdditionalOptions) != "" {
				extraOpts = append(extraOpts, clCompile.AdditionalOptions)
			}
		}
	}

	return strings.Join(extraIncludes, ";"),
		strings.Join(extraDefs, ";"),
		strings.Join(extraOpts, ";")
}

// RemoveBadOptions 移除%(AdditionalOptions)引用
func RemoveBadOptions(opts string) string {
	for _, bad := range badOpts {
		opts = strings.Replace(opts, bad, "", -1)
	}
	return opts
}

func RemoveBadInclude(include string) string {
	for _, bad := range badInclude {
		include = strings.Replace(include, bad, "", -1)
	}
	// 移除可能产生的多余分号
	for strings.Contains(include, ";;") {
		include = strings.Replace(include, ";;", ";", -1)
	}
	// 移除开头和结尾的分号
	include = strings.Trim(include, ";")
	return include
}

func RemoveBadDefinition(def string) string {
	for _, bad := range badDef {
		def = strings.Replace(def, bad, "", -1)
	}
	// 移除可能产生的多余分号
	for strings.Contains(def, ";;") {
		def = strings.Replace(def, ";;", ";", -1)
	}
	// 移除开头和结尾的分号
	def = strings.Trim(def, ";")
	return def
}

// 处理Conan等包管理器的路径
func ProcessConanPaths(includeDirs string) string {
	// 常见的Conan路径模式
	conanPatterns := []string{
		"conan",
		"vcpkg",
		"packages",
	}

	// 如果包含Conan相关路径，保留它们
	for _, pattern := range conanPatterns {
		if strings.Contains(strings.ToLower(includeDirs), pattern) {
			return includeDirs
		}
	}
	return includeDirs
}

// 合并多个分号分隔的字符串列表
func MergeSemicolonSeparatedLists(lists ...string) string {
	var allItems []string
	for _, list := range lists {
		if strings.TrimSpace(list) != "" {
			// 分割分号分隔的项
			parts := strings.Split(list, ";")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" && part != "." {
					allItems = append(allItems, part)
				}
			}
		}
	}
	return strings.Join(allItems, ";")
}

// 合并多个include目录字符串（为了保持向后兼容）
func MergeIncludeDirectories(dirs ...string) string {
	return MergeSemicolonSeparatedLists(dirs...)
}

// 支持PropertyGroup和Import元素
type PropertyGroup struct {
	XMLName                      xml.Name `xml:"PropertyGroup"`
	Condition                    string   `xml:"Condition,attr"`
	Label                        string   `xml:"Label,attr"`
	AdditionalIncludeDirectories string   `xml:"AdditionalIncludeDirectories"`
	IncludeDirectories           string   `xml:"IncludeDirectories"`
}

type Import struct {
	XMLName   xml.Name `xml:"Import"`
	Project   string   `xml:"Project,attr"`
	Condition string   `xml:"Condition,attr"`
}
