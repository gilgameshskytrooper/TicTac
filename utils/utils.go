// utils is the package that defines the various subroutines used by Prometheus. they are all functions.
package utils

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/go-playground/colors.v1"

	"prometheus/structs"
)

//Taking in the IP as a string as the argument, write the IP address to ./public/json/ip to use when the program is restarted
func WriteIP(arg string) {
	writebuf := []byte(arg)
	err := ioutil.WriteFile(Pwd()+"/public/json/ip", writebuf, 0644)
	if err != nil {
		fmt.Println("ERROR WriteIP()")
	}
}

//Call is used to execute complex pipes to filter out the wlan0 IP address of the Pi via ifconfig, awk, and cut
// It is called by Execute once organizes all the separate components of the pipe command
func Call(stack []*exec.Cmd, pipes []*io.PipeWriter) (err error) {
	if stack[0].Process == nil {
		if err = stack[0].Start(); err != nil {
			return err
		}
	}
	if len(stack) > 1 {
		if err = stack[1].Start(); err != nil {
			return err
		}
		defer func() {
			if err == nil {
				pipes[0].Close()
				err = Call(stack[1:], pipes[1:])
			}
		}()
	}
	return stack[0].Wait()
}

//Execute is used to execute complex pipes to filter out the wlan0 IP address of the Pi via ifconfig, awk, and cut
func Execute(output_buffer *bytes.Buffer, stack ...*exec.Cmd) (err error) {
	var error_buffer bytes.Buffer
	pipe_stack := make([]*io.PipeWriter, len(stack)-1)
	i := 0
	for ; i < len(stack)-1; i++ {
		stdin_pipe, stdout_pipe := io.Pipe()
		stack[i].Stdout = stdout_pipe
		stack[i].Stderr = &error_buffer
		stack[i+1].Stdin = stdin_pipe
		pipe_stack[i] = stdout_pipe
	}
	stack[i].Stdout = output_buffer
	stack[i].Stderr = &error_buffer

	if err := Call(stack, pipe_stack); err != nil {
		fmt.Println(string(error_buffer.Bytes()), err)
	}
	return err
}

// Note, if you have a command that needs single quotes such as "awk 'NR==1{print $2}'", make sure you don't include the single quotes (or even escaped double quotes). I'm not sure how, but the code automatically figures out when you need single quotes and fixes it during the execution
func ExampleExecute() {
	var b bytes.Buffer
	var str string
	if err := Execute(&b,
		//Since piping commands are a bit of a pain, using the above functions Call() and Execute(), execute "/sbin/ifconfig wlan0 | grep 'inet addr:' | cut -d -f2 | awk '{print $1}'"
		exec.Command("command1", "flag1", "flag2"),
		exec.Command("command2", "flag1"),
		exec.Command("command3"),
	); err != nil {
		fmt.Println(err)
	}
	str = b.String()
	regex, err := regexp.Compile("\n")
	if err != nil {
		fmt.Println("ERROR")
	}
	str = regex.ReplaceAllString(str, "")
	fmt.Println(strings.TrimSpace(str))
}

// GetIP returns the current wlan0 address as a string
// This is basically running the following command in shell: "ifconfig wlan0 | grep inet | awk 'NR==1{print $2}'" and returning the output as a string
func GetIP() string {
	var b bytes.Buffer
	var str string
	if err := Execute(&b,
		exec.Command("ifconfig", "wlan0"),
		exec.Command("grep", "inet"),
		exec.Command("awk", "NR==1{print $2}"),
	); err != nil {
		fmt.Println(err)
	}
	str = b.String()
	regex, err := regexp.Compile("\n")
	if err != nil {
		fmt.Println("ERROR GetIP()")
	}
	str = regex.ReplaceAllString(str, "")
	return strings.TrimSpace(str)
}

//GetIPFromFile reads the IP from the file, "./public/json/ip", return it as a string
func GetIPFromFile() string {
	content, err := ioutil.ReadFile(Pwd() + "/public/json/ip")
	if err != nil {
		fmt.Println("ERROR GetIPFromFile()")
	}
	lines := strings.Split(string(content), "\n")
	return lines[0]
}

func UseCustomSoundCard() bool {
	content, err := ioutil.ReadFile(Pwd() + "/public/json/customsoundcard")
	if err != nil {
		fmt.Println("ERROR UseCustomSoundCard()")
	}
	lines := strings.Split(string(content), "\n")
	if lines[0] == "true" {
		return true
	} else {
		return false
	}
}

// GetEnableEmail reads the user preference of whether or not they want to be emailed when Prometheus detects a change in IP.
func GetEnableEmail() bool {
	content, err := ioutil.ReadFile(Pwd() + "/public/json/enableemail")
	if err != nil {
		fmt.Println("ERROR GetEnableEmail()")
	}
	lines := strings.Split(string(content), "\n")
	if lines[0] == "True" {
		return true
	} else {
		return false
	}
}

//GetEmail gets the email from "./public/json/email" to be used if the user has a dynamically assigned IP, and the IP changes from before
func GetEmail() string {
	content, err := ioutil.ReadFile(Pwd() + "/public/json/email")
	if err != nil {
		fmt.Println("ERROR GetEmail()")
	}
	lines := strings.Split(string(content), "\n")
	return (lines[0])
}

//Function that checks to see if the current IP matches the IP string currently registered.
//If the old IP and the new IP don't match, send the user an email notifying them of this change. Please change the stored at ./public/json/enableemail to prevent this from happening (via the web interface)
func CheckIPChange() {
	if GetIPFromFile() != GetIP() {
		WriteIP(GetIP())
		sendemail := exec.Command("email/prometheusemail", GetEmail(), GetIP())
		sendemailerror := sendemail.Run()
		if sendemailerror != nil {
			fmt.Println("failed to send email")
		}
		//Account from which Prometheus sends an email from.
	}
}

//Since the Sound and Vibration variables are stored as "on" or "off" in the alarms.json file, this is a simple function that converts a boolean to the "on"/"off" string
func convertBooltoString(arg bool) string {
	if arg {
		return "on"
	} else {
		return "off"
	}
}

//WriteBackJson writes back the correct alarm configurations to ./public/json/alarms.json so that the information can be retrieved when ./main is restarted
func WriteBackJson(Alarm1 structs.Alarm, Alarm2 structs.Alarm, Alarm3 structs.Alarm, Alarm4 structs.Alarm, filepath string) {
	content := []byte("[{\"name\":\"" + Alarm1.Name + "\",\"time\":\"" + Alarm1.Alarmtime + "\",\"sound\":\"" + convertBooltoString(Alarm1.Sound) + "\",\"vibration\":\"" + convertBooltoString(Alarm1.Vibration) + "\"},\n{\"name\":\"" + Alarm2.Name + "\",\"time\":\"" + Alarm2.Alarmtime + "\",\"sound\":\"" + convertBooltoString(Alarm2.Sound) + "\",\"vibration\":\"" + convertBooltoString(Alarm2.Vibration) + "\"},\n{\"name\":\"" + Alarm3.Name + "\",\"time\":\"" + Alarm3.Alarmtime + "\",\"sound\":\"" + convertBooltoString(Alarm3.Sound) + "\",\"vibration\":\"" + convertBooltoString(Alarm3.Vibration) + "\"},\n{\"name\":\"" + Alarm4.Name + "\",\"time\":\"" + Alarm4.Alarmtime + "\",\"sound\":\"" + convertBooltoString(Alarm4.Sound) + "\",\"vibration\":\"" + convertBooltoString(Alarm4.Vibration) + "\"}]")
	err := ioutil.WriteFile(filepath, content, 0644)
	if err != nil {
		fmt.Println("Error writing back JSON alarm file for " + filepath)
	}
}

// Pwd finds the directory of the main process (which would be ../) so that Prometheus can find ../public
// Mainly, this is necessary so that Prometheus can be started in rc.local. The directory becomes relative to the root when started as a startup process. Hence, the ./public folder will no longer be locatable through relative positioning. Pwd ensures you don't have to hardcode the path of the program directory.
func Pwd() string {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		fmt.Println(err)
	}
	return dir
}

// RestartIfNoIP is a function that restarts the Network if not network is detected
func RestartNetwork() {
	_, err := http.Get("http://google.com")
	if err != nil {
		ifdown := exec.Command("ifdown", "wlan0")
		ifdownerror := ifdown.Run()
		if ifdownerror != nil {
			fmt.Println("ifdown wlan0 command failed")
		}
		time.Sleep(time.Second * 5)
		ifup := exec.Command("ifup", "wlan0")
		ifuperror := ifup.Run()
		if ifuperror != nil {
			fmt.Println("ifup wlan0 command failed")
			go RestartNetwork()
		}
	}
}

// WriteEnableEmail write back the information about whether or not the user wants to be notified of IP change to ../public/json/enableemail for data persistence
func WriteEnableEmail(arg string) {
	content := []byte(arg)
	err := ioutil.WriteFile(Pwd()+"/public/json/enableemail", content, 0644)
	if err != nil {
		fmt.Println("Error writing back enableemail file for " + Pwd() + "/public/json/enableemail")
	}

}

// WriteEmail writes back the new user supplied email to ../public/json/email for data persistence
func WriteEmail(arg string) {
	content := []byte(arg)
	err := ioutil.WriteFile(Pwd()+"/public/json/email", content, 0644)
	if err != nil {
		fmt.Println("Error writing back enableemail file for " + Pwd() + "/public/json/enableemail")
	}

}

// Checks to see if there is a running instance of shairport-sync
func CheckShairportRunning() bool {
	var b bytes.Buffer
	var str string
	if err := Execute(&b,
		exec.Command("ps", "aux"),
		exec.Command("grep", "shairport"),
		exec.Command("awk", "NR==1{print $NF}"),
	); err != nil {
		fmt.Println(err.Error())
	}
	str = b.String()
	// doing ps grep | grep shairport | awk 'NR==1{print $NF}' will give the first process with the name shairport-sync
	// If shairport-sync is indeed running, then there will be 2 rows: the first one will be the actual process (which will be named shairport-sync). the second process is the ps aux | grep .... process itself, and it will be named whatever value we passed to grep (in this cased, it will be named shairport)
	if strings.TrimSpace(str) == "shair" {
		return false
	} else {
		return true
	}
}

// Function that kills a running instance of shairport-sync if it is running
func KillShairportSync() {
	if CheckShairportRunning() {
		var b bytes.Buffer
		var str string
		if err := Execute(&b,
			exec.Command("ps", "aux"),
			exec.Command("grep", "shairport"),
			exec.Command("awk", "NR==1{print $2}"),
		); err != nil {
			fmt.Println(err)
		}
		str = b.String()
		killshairport := exec.Command("kill", strings.TrimSpace(str))
		killshairporterror := killshairport.Run()
		if killshairporterror != nil {
			fmt.Println("Could not kill shairport-sync")
		}
	}
}

// Function which checks whether shairport-sync is installed on the system
func CheckShairportSyncInstalled() bool {
	cmd := exec.Command("which", "shairport-sync")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		fmt.Println("which shairport-sync command failed")
	}
	if strings.TrimSpace(stdout.String()) == "" {
		return false
	} else {
		return true
	}
}

// Function that writes persistent data about whether or not to use the custom cvlc commands (if you are using a custom soundcard, you probably do not want to use alsa instead of the default pulse audio)
func WriteCustomSoundCard(arg string) {
	content := []byte(arg)
	err := ioutil.WriteFile(Pwd()+"/public/json/customsoundcard", content, 0644)
	if err != nil {
		fmt.Println("Error writing back enableemail file for " + Pwd() + "/public/json/customsoundcard")
	}
}

func ColorUpdate(arg string) (string, string, string) {
	hex, hexerr := colors.ParseHEX(arg)
	if hexerr != nil {
		fmt.Println("ERROR failed to create a color object based on hex color")
	}
	rgb := hex.ToRGB()
	content := []byte(arg)
	err := ioutil.WriteFile(Pwd()+"/public/json/colors", content, 0644)
	if err != nil {
		fmt.Println("Error writing back colors file for " + Pwd() + "/public/json/colors")
	}

	var stringred, stringgreen, stringblue string
	if int(rgb.R) < 10 {
		stringred = "00" + strconv.Itoa(int(rgb.R))
	} else if int(rgb.R) < 100 && int(rgb.R) > 9 {
		stringred = "0" + strconv.Itoa(int(rgb.R))
	} else {
		stringred = strconv.Itoa(int(rgb.R))
	}

	if int(rgb.G) < 10 {
		stringgreen = "00" + strconv.Itoa(int(rgb.G))
	} else if int(rgb.G) < 100 && int(rgb.G) > 9 {
		stringgreen = "0" + strconv.Itoa(int(rgb.G))
	} else {
		stringgreen = strconv.Itoa(int(rgb.G))
	}

	if int(rgb.B) < 10 {
		stringblue = "00" + strconv.Itoa(int(rgb.B))
	} else if int(rgb.B) < 100 && int(rgb.B) > 9 {
		stringblue = "0" + strconv.Itoa(int(rgb.B))
	} else {
		stringblue = strconv.Itoa(int(rgb.B))
	}
	return stringred, stringgreen, stringblue
}

func ColorInitialize() (string, string, string, bool) {

	content1, err1 := ioutil.ReadFile(Pwd() + "/public/json/colors")
	if err1 != nil {
		fmt.Println("ERROR ColorInitialize() read colors JSON file")
	}
	lines1 := strings.Split(string(content1), "\n")
	hex, hexerr := colors.ParseHEX(lines1[0])
	if hexerr != nil {
		fmt.Println("ERROR failed to create a color object based on hex color")
	}
	rgb := hex.ToRGB()

	var stringred, stringgreen, stringblue string
	if int(rgb.R) < 10 {
		stringred = "00" + strconv.Itoa(int(rgb.R))
	} else if int(rgb.R) < 100 && int(rgb.R) > 9 {
		stringred = "0" + strconv.Itoa(int(rgb.R))
	} else {
		stringred = strconv.Itoa(int(rgb.R))
	}

	if int(rgb.G) < 10 {
		stringgreen = "00" + strconv.Itoa(int(rgb.G))
	} else if int(rgb.G) < 100 && int(rgb.G) > 9 {
		stringgreen = "0" + strconv.Itoa(int(rgb.G))
	} else {
		stringgreen = strconv.Itoa(int(rgb.G))
	}

	if int(rgb.B) < 10 {
		stringblue = "00" + strconv.Itoa(int(rgb.B))
	} else if int(rgb.B) < 100 && int(rgb.B) > 9 {
		stringblue = "0" + strconv.Itoa(int(rgb.B))
	} else {
		stringblue = strconv.Itoa(int(rgb.B))
	}

	content2, err2 := ioutil.ReadFile(Pwd() + "/public/json/enableled")
	if err2 != nil {
		fmt.Println("ERROR ColorInitialize() read enableled file")
	}
	lines2 := strings.Split(string(content2), "\n")
	var returnbool bool
	if lines2[0] == "true" {
		returnbool = true
	} else {
		returnbool = false
	}
	return stringred, stringgreen, stringblue, returnbool
}

func WriteEnableLed(arg string) {
	content := []byte(arg)
	err := ioutil.WriteFile(Pwd()+"/public/json/enableled", content, 0644)
	if err != nil {
		fmt.Println("Error writing back enableemail file for " + Pwd() + "/public/json/enableled")
	}
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return true
}

//Function to check whether the alarm has been running for more than 10 minutes
func OverTenMinutes(alarmtime string) bool {
	year, month, day := time.Now().Date()
	var hour int
	var minutes int
	if string([]rune(alarmtime)[0]) == "0" {
		hour, _ = strconv.Atoi(string([]rune(alarmtime)[1:2]))
	} else {
		hour, _ = strconv.Atoi(string([]rune(alarmtime)[0:2]))
	}

	if string([]rune(alarmtime)[3]) == "0" {
		minutes, _ = strconv.Atoi(string([]rune(alarmtime)[4]))
	} else {
		minutes, _ = strconv.Atoi(string([]rune(alarmtime)[3:]))
	}

	timecurrent := time.Now()
	difference := timecurrent.Minute() - time.Date(int(year), month, int(day), hour, minutes, 0, 0, time.Local).Minute()
	if difference == 10 {
		return true
	} else {
		return false
	}
}
