package util

import "github.com/fatih/color"

var cyan = color.New(color.FgCyan)
var boldCyan = cyan.Add(color.Bold)

var yellow = color.New(color.FgHiYellow)
var boldYellow = yellow.Add(color.Bold)

var red = color.New(color.FgRed)
var boldRed = red.Add(color.Bold)

var green = color.New(color.FgGreen)
var boldGreen = green.Add(color.Bold)

func LogWarning(str1 string, str2 string) {
	yellow.Println("Warning : ", str1)
	yellow.Println(str2)
}

func LogSuccess(str1 string, str2 string) {
	green.Println("Success : ", str1)
	green.Println(str2)
}

func LogInfo(str1 string, str2 string) {
	cyan.Println("Info : ", str1)
	cyan.Println(str2)
}

func LogError(str1 string, str2 string) {
	red.Println("Error : ", str1)
	red.Println(str2)
}
