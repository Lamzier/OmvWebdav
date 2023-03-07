package main

import (
	"encoding/json"
	. "fmt"
	"golang.org/x/net/webdav"
	"io"
	"net/http"
	"os"
	"strings"
)

type Userinfo struct { //用户类
	username string
	password string
	group    []string
}

var userMap = make(map[string]Userinfo)
var webdavPath = "./webdav" // webdav目录
var address = ":8080"       //开放端口

/*
判断目录是否存在
*/
func pathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

/*
初始化文件
*/
func initFile() {
	publicPath := webdavPath + "/public" // 公共目录 ， 可读不可改不可执行
	noSafePath := webdavPath + "/noSafe" //不安全目录 , 可读可改
	userPath := webdavPath + "/user"     // 用户目录， 用户可改可读，其他不可改
	if !pathExists(publicPath) {         //没有目录，创建目录
		Println("创建目录：", publicPath)
		err := os.MkdirAll(publicPath, os.ModePerm)
		if err != nil {
			Println("创建目录是失败！", publicPath)
		}
	}
	if !pathExists(noSafePath) { //没有目录，创建目录
		Println("创建目录：", noSafePath)
		err := os.MkdirAll(noSafePath, os.ModePerm)
		if err != nil {
			Println("创建目录是失败！", noSafePath)
		}

	}
	if !pathExists(userPath) { //没有目录，创建目录
		Println("创建目录：", userPath)
		err := os.MkdirAll(userPath, os.ModePerm)
		if err != nil {
			Println("创建目录是失败！", userPath)
		}
	}
	// 遍历user信息
	for username := range userMap {
		userinfoPath := userPath + "/" + username
		if !pathExists(userinfoPath) { //用户目录不存在
			Println("创建目录：", userinfoPath)
			err := os.MkdirAll(userinfoPath, os.ModePerm)
			if err != nil {
				Println("创建目录是失败！", userinfoPath)
			}
		}
	}
}

type ConfigData struct {
	//userMap    map[string]Userinfo //用户信息
	WebdavPath string              `json:"webdavPath"` // webdav目录
	Address    string              `json:"address"`
	UserMap    map[string][]string `json:"userMap"`
}

/*
获取用户信息
*/
func getUserList() {
	open, err := os.Open("./config.json")
	if err != nil {
		Println("配置文件读取失败！" + err.Error())
		return
	}
	defer func(open *os.File) {
		err := open.Close()
		if err != nil {
			Println("打开配置文件失败！" + err.Error())
		}
	}(open) //关闭流

	all, err := io.ReadAll(open)
	if err != nil {
		Println("打开配置文件失败！" + err.Error())
		return
	}
	var configDate ConfigData
	err = json.Unmarshal(all, &configDate)
	if err != nil {
		Println("配置文件json解析失败！" + err.Error())
		return
	}
	//完成读取配置文件
	if len(configDate.WebdavPath) > 0 {
		webdavPath = configDate.WebdavPath
	}
	if len(configDate.Address) > 0 {
		address = configDate.Address
	}
	users := configDate.UserMap
	if len(users) <= 0 {
		userMap["admin"] = Userinfo{
			username: "admin",
			password: "admin",
			group:    []string{"admin"},
		}
	} else { // 有用户
		for username, value := range users {
			if len(value) <= 1 { //不录入信息
				continue
			}
			userMap[username] = Userinfo{
				username: username,
				password: value[0],
				group:    value[1:],
			}
		}
	}
}

/*
获取网站访问根目录名字
*/
func getUriNames(requestURI string) []string {
	uriNames := strings.Split(requestURI, "/")
	if len(uriNames) <= 1 { //访问的是 "/"
		return nil
	}
	uriNames = uriNames[1:]
	return uriNames
}

/*
判断是否是操作类操作，例如增删改移动
*/
func isOperateMethod(method string) bool {
	var operateMethod = []string{
		"MKCOL", "DELETE", "PUT", "MOVE",
	}
	for i := 0; i < len(operateMethod); i++ {
		temp := operateMethod[i]
		if strings.EqualFold(temp, method) {
			return true
		}
	}
	return false
}

/*
验证用户信息
成功与否 用户信息
*/
func getUserinfo(username string, password string) (bool, Userinfo) {
	userinfo := userMap[username]
	if len(userinfo.username) <= 0 { //找不到用户
		return false, Userinfo{
			username: "",
			password: "",
			group:    nil,
		}
	}
	//找到用户了比对密码
	if userinfo.password != password {
		return false, Userinfo{
			username: "",
			password: "",
			group:    nil,
		}
	}
	//密码正确
	return true, userinfo
}

/*
是否包含用户组
*/
func hasGroup(userinfo Userinfo, group string) bool {
	groups := userinfo.group
	for i := 0; i < len(groups); i++ {
		temp := groups[i]
		if temp == group {
			return true
		}
	}
	return false
}

/*
启动webdav
*/
func createWebDav() {
	Println("WebDav正在启动")
	fs := &webdav.Handler{
		FileSystem: webdav.Dir(webdavPath),
		LockSystem: webdav.NewMemLS(),
	}
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		//先判断是否登陆
		username, password, ok := request.BasicAuth()
		if !ok {
			writer.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		success, userinfo := getUserinfo(username, password)
		if success != true { //账号不对
			http.Error(writer, "WebDAV: need authorized!", http.StatusUnauthorized)
			return
		}
		//先判断路径
		uriNames := getUriNames(request.RequestURI)
		method := request.Method
		if uriNames[0] == "public" { //家庭用户限制写入
			if isOperateMethod(method) { //是操作方法 ， 需要admin
				if !hasGroup(userinfo, "admin") { // 不是管理员
					http.Error(writer, "WebDAV: insufficient privilege!", http.StatusUnauthorized)
					return
				}
			}
		} else if uriNames[0] == "user" && len(uriNames) >= 2 { //用户数据文件
			if isOperateMethod(method) && !hasGroup(userinfo, "admin") { //操作方法 ，不是管理员
				if uriNames[1] != username { //不是用户自己的用户文件
					http.Error(writer, "WebDAV: insufficient privilege!", http.StatusUnauthorized)
					return
				}
			}
		} else if uriNames[0] == "noSafe" { // 权限全部放行
		} else { // 根目录的操作
			if isOperateMethod(method) && !hasGroup(userinfo, "admin") { //操作方法 ，不是管理员
				http.Error(writer, "WebDAV: insufficient privilege!", http.StatusUnauthorized)
				return

			}
		}
		fs.ServeHTTP(writer, request)
	})
	err := http.ListenAndServe(address, nil)
	if err != nil {
		Println("Webdav异常退出！" + err.Error())
		return
	}
}

/*
主函数
*/
func main() {
	getUserList()  // 获取用户信息
	initFile()     //初始化文件
	createWebDav() // 启动webdav
}
