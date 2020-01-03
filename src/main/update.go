package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/btcsuite/winsvc/mgr"
	"github.com/chai2010/winsvc"
	"golang.org/x/sys/windows"

)

const (
	Update_Continue = 0
	Update_Stop     = 1
)

//更新程序结构体
type UpdateProgram struct {
	author           string
	exe_version      string
	source_file      map[string]string //文件名 + 文件完整路径
	source_exe_file  string            //源文件exe路径
	target_dir       map[string]string //serverID + 目标文件路径
	target_exe_file  map[string]string //serverID + 目标exe路径
	server_type      string
	server_prefix    string
	backup_file_num  int
	update_stop_flag int
}

func NewUpdateProgram() *UpdateProgram {
	return &UpdateProgram{}
}

//根据配置进行加载
func (up *UpdateProgram) Load(upcfg *UpdateCfg) error {
	PthSep := string(os.PathSeparator)

	up.author = upcfg.author
	up.exe_version = upcfg.exe_version
	up.server_type = upcfg.server_type
	up.server_prefix = upcfg.server_prefix
	up.source_exe_file = upcfg.source_dir + PthSep + upcfg.source_exe_name
	up.backup_file_num = upcfg.backup_file_num
	up.update_stop_flag = upcfg.update_stop_flag

	up.source_file = make(map[string]string, 0)
	up.target_dir = make(map[string]string, 0)
	up.target_exe_file = make(map[string]string, 0)

	//根据源目录配置得出需要更新哪些文件
	suffix := strings.Split(upcfg.source_file_suffix, ",")
	if filelist, err := GetFiles(upcfg.source_dir, suffix, true); err == nil {
		for _, v := range filelist {
			if str, err := GetFileNameByPath(v); err == nil {
				up.source_file[str] = v
			} else {
				logU.ErrorDoo(err)
			}
		}
	}

	//根据目标目录配置得出需要更新的目标目录文件夹
	up.target_dir, _ = GetCurDirList(upcfg.target_dir, upcfg.server_type, upcfg.not_update_serverid)
	for k, v := range up.target_dir {
		up.target_exe_file[k] = v + PthSep + up.server_prefix + k + ".exe"
	}

	return nil
}

//StartUpdate 更新文件开始,如果某一个文件更新失败即停止更新后续的，如果某个文件更新后启动失败也同样停止更新后续的
func (up *UpdateProgram) StartUpdate() (serverName []string) {

	PthSep := string(os.PathSeparator)
	var success int = 0
	var fail int = 0
	//轮询一遍目标目录,进行文件更新
	for k, v := range up.target_dir {

		if _, ok := up.target_exe_file[k]; !ok {
			logU.InfoDoo("serverID:", k, " not exist correspond exe file")
			fail++
			continue
		}
		curName := up.target_exe_file[k]
		renName := GetNotDittoFileName(v, up.server_prefix+k, up.author, ".exe")

		//如果目标的exe文件存在就先进行重命名
		if b := FileIsExisted(curName); b {
			err := os.Rename(curName, renName)
			if err != nil {
				logU.ErrorDoo("Rename file err: ", err, " curName:", curName, " desName:", renName)
				fail++
				continue
			}
		}

		//拷贝文件
		for name, f := range up.source_file {
			//除了exe文件外先把目标的文件进行重命名
			if !strings.HasSuffix(f, ".exe") {
				rn := GetNotDittoFileName(v, GetFileNamePrefixByPath(name), up.author, GetFileNameSuffixByPath(name))
				cn := v + PthSep + name
				if b := FileIsExisted(cn); b {
					err := os.Rename(cn, rn)
					if err != nil {
						logU.ErrorDoo("Rename file err: ", err, " curName:", curName, " desName:", rn)
					}
				}

				//最多保留up.backup_file_num个会覆盖的目的文件,多余的删除
				if dstName, err := GetFileNameByPath(f); err == nil {
					ClearBackupFile(v, dstName, []string{GetFileNameSuffixByPath(name)}, up.backup_file_num)
				}

			}

			err := CopyFile(v, f)
			if err != nil {
				logU.ErrorDoo("CopyFile file err: ", err, " srcPath:", f, " desDir:", v)
				continue
			}
		}

		//拷贝文件结束后需要对exe程序进行重命名为对应服务的名字
		if exeName, err := GetFileNameByPath(up.source_exe_file); err == nil {
			dstExePath := v + PthSep + exeName
			err = os.Rename(dstExePath, curName)
			if err != nil {
				logU.ErrorDoo("Rename file err: ", err, " curName:", dstExePath, " desName:", curName)
				fail++
				continue
			}
		}

		//获取更新后的exe文件的版本号,并判断是否更新成功
		fi := fileInfo{FilePath: up.target_exe_file[k]}
		fi.GetExeVersion()
		if fi.Version == up.exe_version {
			//更新成功进行多余备份文件处理，最多保留up.backup_file_num个exe文件,多余的删除
			ClearBackupFile(v, up.server_prefix+k+".exe", []string{"exe"}, up.backup_file_num)

			//重启服务，内部会等待直到服务启动或者启动超时
			if !RestartServer(up.server_prefix + k) {
				logU.ErrorDoo("RestartServer:", up.server_prefix+k, "fail please check:", up.target_exe_file[k])
				fail++
				if up.update_stop_flag == Update_Stop {
					goto errorEnd
				} else if up.update_stop_flag == Update_Continue {
					continue
				} else {
					logU.ErrorDoo("don't know update_stop_flag", up.update_stop_flag)
				}
			}

			//存储更新成功的程序的服务名
			serverName = append(serverName, up.server_prefix+k)
			logU.InfoDoo("File:", up.target_exe_file[k], "update success and restart success version is:", fi.Version)
		} else {
			logU.ErrorDoo("File:", up.target_exe_file[k], "update fail version is:", fi.Version, "please check exe_version is match")
			fail++
			goto errorEnd
		}

		success++
		logU.InfoDoo("Update progress[success:", success, "fail:", fail, "total:", len(up.target_dir))
	}

	return
errorEnd:
	logU.InfoDoo("Update progress[success:", success, "fail:", fail, "total:", len(up.target_dir))
	return
}

func RestartServer(name string) bool {
	servicePidPre, _ := GetServicePID(name)     //先查询服务PID
	statuePre, err := winsvc.QueryService(name) //先查询服务状态
	if err != nil {
		logU.ErrorDoo("QueryService", name, "fail:", err)
		return false
	}

	//重启服务
	if statuePre == "Running" {
		if err := winsvc.StopService(name); err != nil {
			logU.ErrorDoo("StopService", name, "fail:", err)
		}

		if err := winsvc.StartService(name); err != nil {
			logU.ErrorDoo("StartService1", name, "fail:", err)
		}
	} else {
		if err := winsvc.StartService(name); err != nil {
			logU.ErrorDoo("StartService2", name, "fail:", err)
		}
	}

	//重新启动成功的标志是PID前后不一样并且服务是运行状态的
	servicePidAfter, _ := GetServicePID(name)
	statueAfter, _ := winsvc.QueryService(name)
	if servicePidPre != servicePidAfter && statueAfter == "Running" {
		return true
	}

	return false
}

//获取不重复的文件名
func GetNotDittoFileName(dir, prefix, midWord, suffix string) string {
	PthSep := string(os.PathSeparator)
	t := time.Now().Format("20060102")
	for i := 0; ; i++ {
		file := dir + PthSep + prefix + "(" + midWord + t + "_" + strconv.Itoa(i) + ")" + suffix
		if !FileIsExisted(file) {
			return file
		}
	}

	return "GetNotDittoFileNameError"
}

//文件属性,用于根据修改时间排序文件
type FileAttr struct {
	path       string
	modifyTime int64
}

//为*FileAttr添加String()方法，便于输出
func (fa *FileAttr) String() string {
	return fmt.Sprintf("( %s,%d )", fa.path, fa.modifyTime)
}

type FileAttrList []*FileAttr

func (list FileAttrList) Len() int {
	return len(list)
}

func (list FileAttrList) Less(i, j int) bool {
	if list[i].modifyTime < list[j].modifyTime {
		return true
	} else if list[i].modifyTime > list[j].modifyTime {
		return false
	} else {
		return list[i].path < list[j].path
	}
}

func (list FileAttrList) Swap(i, j int) {
	var temp *FileAttr = list[i]
	list[i] = list[j]
	list[j] = temp
}

//清理备份文件(对于某个目录下的某种文件类型,除了当前在使用的那个外，最多能保留多少个，多余的就删除掉)
func ClearBackupFile(fileDir, fileName string, suffixs []string, num int) {
	PthSep := string(os.PathSeparator)
	needOpList := make([]*FileAttr, 0)
	retainFile := fileDir + PthSep + fileName
	if curFileList, err := GetFiles(fileDir, suffixs, false); err == nil {
		if len(curFileList) > num {
			for _, v := range curFileList {
				if retainFile != v {
					needOpList = append(needOpList, &FileAttr{v, GetFileModTime(v)})
				}
			}
		}

		//根据修改时间对文件进行排序
		sort.Sort(FileAttrList(needOpList))

		//删除掉创建时间比较久的文件
		if len(needOpList) > num {
			deleteNum := len(needOpList) - num
			for i := 0; i < deleteNum; i++ {
				os.Remove(needOpList[i].path)
			}
		}

	}
}

//获取文件修改时间 返回unix时间戳
func GetFileModTime(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		logU.ErrorDoo("GetFileModTime open file error")
		return time.Now().Unix()
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		logU.ErrorDoo("GetFileModTime stat fileinfo error")
		return time.Now().Unix()
	}

	return fi.ModTime().Unix()
}

//把文件从源路径复制到目标目录下
func CopyFile(dstFileDir string, srcFilePath string) (err error) {
	srcFile, err := os.Open(srcFilePath)
	if err != nil {
		logU.ErrorDoo("打开源文件错误，错误信息", err, srcFilePath)
		return err
	}
	defer srcFile.Close()

	var dstName string
	if dstName, err = GetFileNameByPath(srcFilePath); err != nil {
		logU.ErrorDoo(err)
		return err
	}

	PthSep := string(os.PathSeparator)
	//获取源文件的权限
	fi, _ := srcFile.Stat()
	perm := fi.Mode()

	dstFile, err := os.OpenFile(dstFileDir+PthSep+dstName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		logU.ErrorDoo("打开目标文件错误，错误信息", err, dstName)
		return err
	}
	defer dstFile.Close()

	buf := make([]byte, 1024*1024)
	for {
		n, err := srcFile.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}
		if _, err := dstFile.Write(buf[:n]); err != nil {
			return err
		}
	}

	return nil
}

//获取当前路径下的目录
func GetCurDirList(path, server_type, filter string) (dirmap map[string]string, err error) {
	dir, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}

	PthSep := string(os.PathSeparator)
	dirmap = make(map[string]string, 0)

	for _, fi := range dir {
		if fi.IsDir() {

			//过滤掉不需要更新的子服务
			if strings.Contains(filter, fi.Name()) {
				continue
			}

			//获取子目录
			subDir, err := ioutil.ReadDir(path + PthSep + fi.Name())
			if err != nil {
				logU.ErrorDoo(err)
				continue
			}

			count := 0
			subDirName := ""
			for _, subFi := range subDir {
				if subFi.IsDir() && strings.Contains(subFi.Name(), server_type) {
					count++
					subDirName = subFi.Name()
				}
			}

			//只允许存在一个该类型的子文件如E:\tradesystem\server_id\trade4 不能还存在E:\tradesystem\server_id\MT4这种的
			if count == 1 {
				dirmap[fi.Name()] = path + PthSep + fi.Name() + PthSep + subDirName
			} else {
				logU.ErrorDoo("Please check in the path", path+PthSep+fi.Name(), "contians the server_type", server_type, "dir")
			}

		}
	}

	return dirmap, nil
}

//GetFiles 获取指定目录下的所有文件,(all为true表示包含子目录下的文件 否则只是单前目录下的文件)
func GetFiles(dirPth string, suffixs []string, all bool) (files []string, err error) {
	dir, err := ioutil.ReadDir(dirPth)
	if err != nil {
		return nil, err
	}

	PthSep := string(os.PathSeparator)
	//suffix = strings.ToUpper(suffix) //忽略后缀匹配的大小写

	for _, fi := range dir {
		if fi.IsDir() && all { // 目录, 递归遍历
			if ls, err := GetFiles(dirPth+PthSep+fi.Name(), suffixs, all); err == nil {
				files = append(files, ls...)
			}
		} else {
			// 过滤指定格式
			for _, suffix := range suffixs {
				ok := strings.HasSuffix(fi.Name(), suffix)
				if ok {
					files = append(files, dirPth+PthSep+fi.Name())
					break
				}
			}
		}
	}

	return files, nil
}

//通过路径获取文件名
func GetFileNameByPath(path string) (string, error) {
	PthSep := string(os.PathSeparator)

	strList := strings.Split(path, PthSep)

	if len(strList) > 0 {
		return strList[len(strList)-1], nil
	} else {
		return "", fmt.Errorf("path error %s", path)
	}
}

func GetFileNameSuffixByPath(path string) string {
	index := strings.LastIndex(path, ".")
	if index != -1 {
		return path[index:]
	}
	return ".unknow"
}

func GetFileNamePrefixByPath(path string) string {
	index := strings.LastIndex(path, ".")
	if index != -1 {
		return path[:index]
	}
	return "unknow"
}

//判断文件是否存在
func FileIsExisted(filename string) bool {
	existed := true
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		existed = false
	}
	return existed
}

//GetServicePID 获取服务的PID
func GetServicePID(serviceName string) (uint32, error) {
	manager, err := mgr.Connect()
	if err != nil {
		logU.ErrorDoo("Open service manager err", err)
		return 0, err
	}
	defer manager.Disconnect()

	var needBuf uint32
	var serviceNum uint32
	if err := windows.EnumServicesStatusEx(windows.Handle(manager.Handle), windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL, nil, 0, &needBuf, &serviceNum, nil, nil); err != nil {
		//这里会报错但是不影响到获取needBuf
	}

	services := make([]byte, needBuf)
	if err := windows.EnumServicesStatusEx(windows.Handle(manager.Handle), windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL, (*byte)(unsafe.Pointer(&services[0])), needBuf, &needBuf, &serviceNum, nil, nil); err != nil {
		logU.ErrorDoo("EnumServicesStatusEx get part service list fail err:", err)
		return 0, err
	}

	var sizeWinSer windows.ENUM_SERVICE_STATUS_PROCESS
	iter := uintptr(unsafe.Pointer(&services[0]))
	for i := uint32(0); i < serviceNum; i++ {
		var data = (*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(iter))
		iter = uintptr(unsafe.Pointer(iter + unsafe.Sizeof(sizeWinSer)))
		//fmt.Printf("Service Name: %s - Display Name: %s - %#v\r\n", syscall.UTF16ToString((*[4096]uint16)(unsafe.Pointer(data.ServiceName))[:]), syscall.UTF16ToString((*[4096]uint16)(unsafe.Pointer(data.DisplayName))[:]), data.ServiceStatusProcess)
		name := syscall.UTF16ToString((*[100]uint16)(unsafe.Pointer(data.ServiceName))[:])

		if serviceName == name {
			return data.ServiceStatusProcess.ProcessId, nil
		}

	}

	return 0, fmt.Errorf("no found service %s info", serviceName)
}

func RemoveService(serviceName []string) {
	for _, s := range serviceName {
		if err := winsvc.RemoveService(s); err != nil {
			logU.ErrorDoo("RemoveService", s, "fail:", err)
		}
	}

}
