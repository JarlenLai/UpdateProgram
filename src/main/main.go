package main

import (
	"bufio"
	"fmt"
	"logdoo"
	"os"
	"path/filepath"

)

var logU = logdoo.NewLogger() //log函数

//初始化
func init() {
	if logPath, err := CreateLogDir("updateLog"); err == nil {
		var logUF = logdoo.NewDayLogHandle(logPath, 800)
		var logUC = logdoo.NewConsoleHandler()
		logU.SetHandlers(logUF, logUC)
	}
}

//函数入口
func main() {

	//获取配置目录
	cfgpath, err := GetCfgPath()
	if err != nil {
		logU.ErrorDoo(err)
		return
	}

	updateCfg := NewUpdateCfg()
	updateCfg.Load(cfgpath)

	updateProgram := NewUpdateProgram()
	updateProgram.Load(updateCfg)
	updateProgram.StartUpdate()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("**Update end please check the log to confirm update result**\n\n")
	fmt.Print(">>please input enter to quit\n")
	reader.ReadString('\n')
}

//PathExists 判断路径是否存在
func PathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}

	if os.IsExist(err) {
		return true
	}

	return false
}

//CreateLogDir 创建目录路径
func CreateLogDir(logName string) (string, error) {
	// 获取当前路径
	dir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	logpath := dir + "\\" + logName
	if PathExists(logpath) {
		return logpath, nil
	}

	err := os.Mkdir(logpath, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("os.Mkdir err:%s", err.Error())
	}

	return logpath, nil
}

//GetCfgPath 获取当前配置文件的路径
func GetCfgPath() (string, error) {
	// 获取当前路径
	PthSep := string(os.PathSeparator)

	dir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	cfgpath := dir + PthSep + "config" + PthSep + "config.ini"
	if PathExists(cfgpath) {
		return cfgpath, nil
	}

	if !PathExists(dir) {
		os.MkdirAll(dir, os.ModePerm)
	}

	if !PathExists(dir + PthSep + "config") {
		os.Mkdir(dir+PthSep+"config", os.ModePerm)
	}

	if !PathExists(cfgpath) {
		file, err := os.Create(cfgpath)
		if err != nil {
			return cfgpath, fmt.Errorf("config %s not exists and create it fail:%s", cfgpath, err)
		}
		defer file.Close()

		initContent := "#[Signature] 签名配置信息(author用于记录更新人,exe_version表示服务需升级到的版本用于判断服务是否更新成功)\r\n" +
			"[Signature]\r\nauthor=jarlen\r\nexe_version=1.0.0.1\r\n\n" +

			"#[Update_Cfg] 更新配置\r\n" +
			"##source_dir 源目录(更新文件的来源配合 source_file_suffix 使用表示具体更新该目录下的哪些类型的文件)\r\n" +
			"#source_exe_name 源目录下的主程序文件名称\r\n" +
			"#target_dir 更新到的目标目录（程序会遍历该目录获取所有的ServerID目录）再具体配合server_type 和 server_prefix 结合拼接成所有所有要更新的子服务目录\r\n" +
			"#server_type 取值4或者5（代表是更新该serverid的mt5类型还是mt4类型）\r\n" +
			"#server_prefix 要更新的服务名称的前缀\r\n" +
			"#not_update_serverid 表示无需更新的serverID（使用,号隔开）,为空则表示全部都更新\r\n" +
			"#backup_file_num 最多保留的备份的个数,多余并且最旧的会被清理掉\r\n" +
			"[Update_Cfg]\r\nsource_dir=\r\nsource_file_suffix=\r\nsource_exe_name=\r\ntarget_dir=\r\nserver_type=\r\nserver_prefix=\r\nnot_update_serverid=\r\nbackup_file_num=\r\n\n"

		file.WriteString(initContent)
	}

	return cfgpath, nil
}
