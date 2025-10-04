package sln

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Sln struct {
	SolutionDir string
	ProjectList []Project
}

func NewSln(path string) (Sln, error) {
	var sln Sln
	var err error

	sln.SolutionDir, err = filepath.Abs(path)
	sln.SolutionDir = filepath.Dir(sln.SolutionDir)
	if err != nil {
		return sln, err
	}
	projectFiles, err := findAllProject(path)
	if err != nil {
		fmt.Println(err)
		return sln, err
	}
	if len(projectFiles) == 0 {
		return sln, errors.New("not found project file")
	}

	for _, path := range projectFiles {
		pro, err := NewProject(filepath.Join(sln.SolutionDir, path))
		if err != nil {
			return sln, err
		}
		sln.ProjectList = append(sln.ProjectList, pro)
	}
	return sln, nil
}

func findAllProject(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return []string{}, err
	}
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	if err != nil {
		return []string{}, err
	}
	re := regexp.MustCompile("[^\"]\"[^\"]+\\.vcxproj\"")
	files := re.FindAllString(string(b), -1)

	var list []string
	for _, v := range files {
		v = strings.Replace(v, "\"", "", -1)
		v = strings.TrimSpace(v)
		list = append(list, v)
	}
	return list, nil
}

func (sln *Sln) CompileCommandsJson(conf string) ([]CompileCommand, error) {
	var cmdList []CompileCommand

	for _, pro := range sln.ProjectList {
		var item CompileCommand

		for _, f := range pro.FindSourceFiles() {
			item.Dir = pro.ProjectDir
			item.File = f

			// 使用增强的配置查找函数
			inc, def, additionalOpts, usingDirs, err := pro.FindConfigEnhanced(conf)
			if err != nil {
				// 如果增强函数失败，回退到原始函数
				inc, def, err = pro.FindConfig(conf)
				if err != nil {
					return cmdList, err
				}
			}

			// 收集ItemGroup中的额外配置
			extraInc, extraDef, extraOpt := pro.FindItemGroupConfigs(conf)

			// 处理SolutionDir环境变量替换
			willReplaceEnv := map[string]string{
				"$(SolutionDir)": sln.SolutionDir,
			}
			for k, v := range willReplaceEnv {
				if strings.Contains(inc, k) {
					inc = strings.Replace(inc, k, v, -1)
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
				// 处理额外配置的环境变量替换
				if strings.Contains(extraInc, k) {
					extraInc = strings.Replace(extraInc, k, v, -1)
				}
				if strings.Contains(extraDef, k) {
					extraDef = strings.Replace(extraDef, k, v, -1)
				}
				if strings.Contains(extraOpt, k) {
					extraOpt = strings.Replace(extraOpt, k, v, -1)
				}
			}

			// 合并所有include目录和定义
			allIncludeDirs := MergeIncludeDirectories(inc, usingDirs, extraInc)
			allDefs := MergeIncludeDirectories(def, extraDef)
			allOpts := MergeIncludeDirectories(additionalOpts, extraOpt)

			// 处理Conan等包管理器路径
			allIncludeDirs = ProcessConanPaths(allIncludeDirs)

			// 清理和格式化参数
			allDefs = RemoveBadDefinition(allDefs)
			allDefs = preappend(allDefs, "-D")

			allIncludeDirs = RemoveBadInclude(allIncludeDirs)
			allIncludeDirs = preappend(allIncludeDirs, "-I")

			// 处理额外编译选项
			allOpts = strings.TrimSpace(allOpts)

			// 构建完整的编译命令
			var cmdParts []string
			cmdParts = append(cmdParts, "clang-cl.exe")
			if strings.TrimSpace(allDefs) != "" {
				cmdParts = append(cmdParts, strings.TrimSpace(allDefs))
			}
			if strings.TrimSpace(allIncludeDirs) != "" {
				cmdParts = append(cmdParts, strings.TrimSpace(allIncludeDirs))
			}
			if allOpts != "" {
				cmdParts = append(cmdParts, allOpts)
			}
			cmdParts = append(cmdParts, "-c", f)

			item.Cmd = strings.Join(cmdParts, " ")
			cmdList = append(cmdList, item)
		}

	}
	return cmdList, nil
}

func preappend(sepedString string, append string) string {
	defList := strings.Split(sepedString, ";")
	var output string

	for _, v := range defList {
		v = append + v + " "
		output += v
	}
	return output
}
