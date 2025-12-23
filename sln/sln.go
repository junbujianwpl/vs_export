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

	// 获取文件扩展名
	ext := strings.ToLower(filepath.Ext(path))

	// 获取文件的绝对路径和所在目录
	absPath, err := filepath.Abs(path)
	if err != nil {
		return sln, err
	}
	sln.SolutionDir = filepath.Dir(absPath)

	if ext == ".sln" {
		// 处理解决方案文件
		projectFiles, err := findAllProject(path)
		if err != nil {
			fmt.Println(err)
			return sln, err
		}
		if len(projectFiles) == 0 {
			return sln, errors.New("not found project file")
		}

		for _, projectPath := range projectFiles {
			pro, err := NewProject(filepath.Join(sln.SolutionDir, projectPath))
			if err != nil {
				return sln, err
			}
			sln.ProjectList = append(sln.ProjectList, pro)
		}
	} else if ext == ".vcxproj" {
		// 直接处理单个项目文件
		pro, err := NewProject(absPath)
		if err != nil {
			return sln, err
		}
		sln.ProjectList = append(sln.ProjectList, pro)
	} else {
		return sln, fmt.Errorf("unsupported file format: %s, only .sln and .vcxproj are supported", ext)
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

// 生成compile_commands.json内容
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

			// 合并所有include目录
			allIncludeDirs := MergeSemicolonSeparatedLists(inc, usingDirs, extraInc)

			// 添加系统include目录（基于MSVC标准路径）
			var systemIncludeDirs []string
			// 检查是否有Visual Studio环境变量
			if vsInstallDir := os.Getenv("VSINSTALLDIR"); vsInstallDir != "" {
				platformToolset := "v143" // 默认使用VS 2022的工具集
				// 尝试从环境变量获取工具集版本
				if toolset := os.Getenv("PlatformToolsetVersion"); toolset != "" {
					platformToolset = toolset
				}

				// 添加MSVC标准库include目录
				systemIncludeDirs = append(systemIncludeDirs, filepath.Join(vsInstallDir, "VC", "Tools", "MSVC", platformToolset, "include"))

				// 添加Windows SDK include目录
				if windowsSdkDir := os.Getenv("WindowsSdkDir"); windowsSdkDir != "" {
					if windowsSdkVersion := os.Getenv("WindowsSdkVersion"); windowsSdkVersion != "" {
						systemIncludeDirs = append(systemIncludeDirs, filepath.Join(windowsSdkDir, "Include", windowsSdkVersion, "um"))
						systemIncludeDirs = append(systemIncludeDirs, filepath.Join(windowsSdkDir, "Include", windowsSdkVersion, "shared"))
						systemIncludeDirs = append(systemIncludeDirs, filepath.Join(windowsSdkDir, "Include", windowsSdkVersion, "winrt"))
						systemIncludeDirs = append(systemIncludeDirs, filepath.Join(windowsSdkDir, "Include", windowsSdkVersion, "cppwinrt"))
					}
				}
			}

			// 如果没有环境变量，添加默认的系统include目录位置
			if len(systemIncludeDirs) == 0 {
				// VS 2022默认安装路径
				systemIncludeDirs = append(systemIncludeDirs, "C:\\Program Files\\Microsoft Visual Studio\\2022\\Community\\VC\\Tools\\MSVC\\14.39.33519\\include")
				systemIncludeDirs = append(systemIncludeDirs, "C:\\Program Files (x86)\\Windows Kits\\10\\Include\\10.0.22621.0\\um")
				systemIncludeDirs = append(systemIncludeDirs, "C:\\Program Files (x86)\\Windows Kits\\10\\Include\\10.0.22621.0\\shared")
				systemIncludeDirs = append(systemIncludeDirs, "C:\\Program Files (x86)\\Windows Kits\\10\\Include\\10.0.22621.0\\winrt")
			}

			// 合并系统include目录
			if len(systemIncludeDirs) > 0 {
				allIncludeDirs = MergeSemicolonSeparatedLists(allIncludeDirs, strings.Join(systemIncludeDirs, ";"))
			}

			// 合并所有宏定义
			allDefs := MergeSemicolonSeparatedLists(def, extraDef)

			// 添加默认的MSVC宏定义
			defaultDefs := []string{
				"WIN32",    // Windows平台
				"_WINDOWS", // Windows应用程序
				"_MBCS",    // 多字节字符集
			}
			// 根据配置添加特定宏
			if strings.Contains(strings.ToLower(conf), "debug") {
				defaultDefs = append(defaultDefs, "_DEBUG", "DEBUG") // Debug配置
			} else {
				defaultDefs = append(defaultDefs, "NDEBUG") // Release配置
			}
			if strings.Contains(strings.ToLower(conf), "win32") {
				defaultDefs = append(defaultDefs, "_WIN32") // 32位平台
			} else if strings.Contains(strings.ToLower(conf), "x64") {
				defaultDefs = append(defaultDefs, "_WIN64") // 64位平台
			}

			// 合并默认宏定义
			if len(defaultDefs) > 0 {
				allDefs = MergeSemicolonSeparatedLists(allDefs, strings.Join(defaultDefs, ";"))
			}

			// 合并额外编译选项
			allOpts := MergeSemicolonSeparatedLists(additionalOpts, extraOpt)

			// 处理Conan等包管理器路径
			allIncludeDirs = ProcessConanPaths(allIncludeDirs)

			// 清理和格式化参数
			allDefs = RemoveBadDefinition(allDefs)
			allDefs = preappend(allDefs, "-D")

			allIncludeDirs = RemoveBadInclude(allIncludeDirs)
			allIncludeDirs = preappend(allIncludeDirs, "-I")

			// 处理额外编译选项
			allOpts = strings.TrimSpace(allOpts)
			// 移除%(AdditionalOptions)宏
			allOpts = RemoveBadOptions(allOpts)

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