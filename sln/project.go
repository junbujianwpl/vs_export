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
	ItemGroup           []ItemGroup           `xml:"ItemGroup"`
	ItemDefinitionGroup []ItemDefinitionGroup `xml:"ItemDefinitionGroup"`
}
type ItemGroup struct {
	XMLName                  xml.Name               `xml:"ItemGroup"`
	Label                    string                 `xml:"Label,attr"`
	ProjectConfigurationList []ProjectConfiguration `xml:"ProjectConfiguration"`
	ClCompileSrc             []ClCompileSrc         `xml:"ClCompile"`
	// 支持ItemGroup中的ClCompile配置
	ClCompileItems []ClCompileItem `xml:"ClCompile"`
}

// ItemGroup中的ClCompile元素
type ClCompileItem struct {
	XMLName                      xml.Name `xml:"ClCompile"`
	Include                      string   `xml:"Include,attr"`
	AdditionalIncludeDirectories string   `xml:"AdditionalIncludeDirectories"`
	PreprocessorDefinitions      string   `xml:"PreprocessorDefinitions"`
	AdditionalOptions            string   `xml:"AdditionalOptions"`
}

type ProjectConfiguration struct {
	XMLName       xml.Name `xml:"ProjectConfiguration"`
	Include       string   `xml:"Include,attr"`
	Configuration string   `xml:"Configuration"`
	Platform      string   `xml:"Platform"`
}

type ItemDefinitionGroup struct {
	XMLName   xml.Name  `xml:"ItemDefinitionGroup"`
	Condition string    `xml:"Condition,attr"`
	ClCompile ClCompile `xml:"ClCompile"`
}

type ClCompile struct {
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

type ClCompileSrc struct {
	XMLName xml.Name `xml:"ClCompile"`
	Include string   `xml:"Include,attr"`
}

type CompileCommand struct {
	Dir  string `json:"directory"`
	Cmd  string `json:"command"`
	File string `json:"file"`
}

var badInclude = []string{
	";%(AdditionalIncludeDirectories)",
	"%(AdditionalIncludeDirectories);",
}
var badDef = []string{
	";%(PreprocessorDefinitions)",
	"%(PreprocessorDefinitions);",
}

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
	fmt.Fprintln(os.Stderr, cfgList)
	if len(cfgList) == 0 {
		return "", "", "", "", errors.New(pro.ProjectPath + ":not found " + conf)
	}
	found := false
	for _, v := range cfgList {
		if v.Include == conf {
			found = true
			break
		}
	}
	if !found {
		return "", "", "", "", errors.New(pro.ProjectPath + ":not found " + conf)
	}
	for _, v := range pro.ItemDefinitionGroup {
		if strings.Contains(v.Condition, conf) {
			cl := v.ClCompile

			vlist := strings.Split(conf, "|")
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
			additionalOpts := cl.AdditionalOptions
			usingDirs := cl.AdditionalUsingDirectories

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
	}
	return "", "", "", "", errors.New("not found " + conf)
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
	fmt.Fprintln(os.Stderr, cfgList)
	if len(cfgList) == 0 {
		return "", "", errors.New(pro.ProjectPath + ":not found " + conf)
	}
	found := false
	for _, v := range cfgList {
		if v.Include == conf {
			found = true
			break
		}
	}
	if !found {
		return "", "", errors.New(pro.ProjectPath + ":not found " + conf)
	}
	for _, v := range pro.ItemDefinitionGroup {
		if strings.Contains(v.Condition, conf) {
			cl := v.ClCompile

			vlist := strings.Split(conf, "|")
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
		for _, inc := range v.ClCompileSrc {
			fileList = append(fileList, inc.Include)
		}
	}
	return fileList
}

// 收集ItemGroup中的额外配置
func (pro *Project) FindItemGroupConfigs(conf string) (string, string, string) {
	var extraIncludes, extraDefs, extraOpts []string

	for _, itemGroup := range pro.ItemGroup {
		for _, clCompile := range itemGroup.ClCompileItems {
			// 检查是否匹配当前配置
			if strings.Contains(clCompile.AdditionalIncludeDirectories, conf) ||
				strings.Contains(clCompile.PreprocessorDefinitions, conf) ||
				strings.Contains(clCompile.AdditionalOptions, conf) {

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
	}

	return strings.Join(extraIncludes, ";"),
		strings.Join(extraDefs, ";"),
		strings.Join(extraOpts, ";")
}

func RemoveBadInclude(include string) string {
	for _, bad := range badInclude {
		include = strings.Replace(include, bad, ";.", -1)
	}
	return include
}

func RemoveBadDefinition(def string) string {
	for _, bad := range badDef {
		def = strings.Replace(def, bad, "", -1)
	}
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

// 合并多个include目录字符串
func MergeIncludeDirectories(dirs ...string) string {
	var allDirs []string
	for _, dir := range dirs {
		if strings.TrimSpace(dir) != "" {
			// 分割分号分隔的路径
			parts := strings.Split(dir, ";")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" && part != "." {
					allDirs = append(allDirs, part)
				}
			}
		}
	}
	return strings.Join(allDirs, ";")
}
