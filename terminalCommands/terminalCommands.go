package terminalCommands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// Sender ---
type Sender struct {
	http        *http.Client
	Loggers     []func(string, string, *string, *string, *CommandAction, *TerminalResponse, error)
	terminalURL *string
}

// TerminalResponse ---
type TerminalResponse struct {
	Id        string                 `json:"id"`
	Result    int32                  `json:"result"`
	Stage     int32                  `json:"stage,omitempty"`
	Errors    []CommandError         `json:"errors,omitempty"`
	Telemetry map[string]interface{} `json:"telemetry,omitempty"`
	Info      map[string]interface{} `json:"info,omitempty"`
	Driver    string                 `json:"driver,omitempty"`
	Device    string                 `json:"device,omitempty"`
	Token     string                 `json:"token,omitempty"`
	Expired   uint64                 `json:"expired,omitempty"`
}

// CommandAction ---
type CommandAction struct {
	Id      string                 `json:"id"`
	Index   uint32                 `json:"index,omitempty"`
	Act     uint32                 `json:"act,omitempty"`
	Ton     uint32                 `json:"ton,omitempty"`
	Toff    uint32                 `json:"toff,omitempty"`
	Args    map[string]interface{} `json:"args,omitempty"`
	Next    []interface{}          `json:"next,omitempty"`
	Result  *int                   `json:"result,omitempty"`
	Timeout uint32                 `json:"timeout,omitempty"`
}

// Command ---
type Command struct {
	Id      string        `json:"id"`
	Target  string        `json:"target"`
	Command CommandAction `json:"command"`
	Timeout int           `json:"timeout"`
}

// CommandError ---
type CommandError struct {
	Code    int32  `json:"code"`
	Message string `json:"message"`
}

const (
	// belka compatible
	guard_error_ign    = 0x000001
	guard_error_park   = 0x000002
	guard_error_doors  = 0x000004
	guard_error_trunk  = 0x000008
	guard_error_space  = 0x000010
	guard_error_hood   = 0x000020
	guard_error_lights = 0x000040
	guard_error_busy   = 0x000080

	// flex extended
	guard_error_sensor    = 0x000100
	guard_error_guard     = 0x000200
	guard_error_can_park  = 0x000400
	guard_error_can_brake = 0x000800
	guard_error_already   = 0x100000
	guard_error_timeout   = 0x200000
	guard_error_disable   = 0x400000
	guard_error_other     = 0x800000

	// common
	command_error_other        = guard_error_other
	command_error_timeout      = guard_error_timeout
	command_error_disable      = guard_error_disable
	command_error_invalid_args = 0x1000000
	command_error_invalid_crc  = 0x2000000
)

// ErrorCodes ---
var ErrorCodes = map[int32]string{
	1100: "TurnOffIgnition",
	1101: "TurnOnParking",
	1102: "CloseDoors",
	1103: "CloseTrunk",
	1104: "VolumeSensor",
	1105: "CloseHood",
	1106: "TurnOffLight",
	1107: "CommandRunning",
	1108: "SensorError",
	1109: "GuardError",
	1110: "BrakeError",
	1111: "CommandDisabled",
	1112: "OtherError",
	-100: "Cancel",
	-101: "ServiceUnavailable",
	-102: "InvalidResponse",
	-103: "ProcessError",
	-1:   "Fail",
	-2:   "Notimpl",
	-3:   "CommandTimeout",
	-4:   "InvalidArg",
	-5:   "InvalidCheksum",
	-6:   "LowData",
	-7:   "InvalidFormat",
	-8:   "Terminate",
	-11:  "CommandTimeout",
	-12:  "CommandIncomplete",
	-13:  "ExpectedResult",
}

// SetError ---
func (response *TerminalResponse) SetError(code int32) {
	if response.Errors == nil {
		response.Errors = make([]CommandError, 0)
	}
	response.Errors = append(response.Errors, CommandError{code, ErrorCodes[code]})
	if code < 0 {
		response.Result = code
	} else {
		response.Result = -1
	}
}

// Run send command request and wait answer
func (sender *Sender) Run(objID, imei, drv string, clientID, userID *string, cmdID *string, action *CommandAction, timeout ...int) (response TerminalResponse) {
	target := imei
	if drv != "" {
		target = drv + ":" + target
	}
	cmd := Command{
		Id:      uuid.New().String(),
		Target:  target,
		Command: *action,
	}
	if len(timeout) > 0 && timeout[0] > 0 {
		cmd.Timeout = timeout[0]
	} else {
		cmd.Timeout = int(action.Timeout)
	}
	b, err := json.Marshal(cmd)
	if err != nil {
		log.Error(err)
		response.Result = -103
	} else {
		r := bytes.NewReader(b)
		resp, err := sender.http.Post(*sender.terminalURL, "application/json", r)
		if err != nil {
			log.Error(err)
			response.SetError(-101)
		} else {
			decoder := json.NewDecoder(resp.Body)
			err := decoder.Decode(&response)
			if err != nil {
				log.Error(err)
				response.SetError(-102)
			} else if response.Result < 0 {
				response.SetError(response.Result)
			} else {
				response.SetBitErrors()
			}
			log.Debug(response)
			log.Info("cmd response ", resp)
		}
	}
	if sender.Loggers != nil && len(sender.Loggers) > 0 {
		var cmd string
		if cmdID != nil {
			cmd = *cmdID
		} else {
			cmd = action.Id
		}
		for _, cb := range sender.Loggers {
			cb(objID, cmd, clientID, userID, action, &response, err)
		}
	}
	return response
}

// Protection ---
func (sender *Sender) Protection(objID, imei, drv string, userID *string, on uint8) TerminalResponse {
	action := CommandAction{
		Id:  "guard",
		Act: uint32(on),
	}

	return sender.Run(objID, imei, drv, nil, userID, nil, &action)
}

// Engine ---
func (sender *Sender) Engine(objID, imei, drv string, userID *string, on uint8) TerminalResponse {
	action := CommandAction{
		Id:  "engine",
		Act: uint32(on),
	}

	return sender.Run(objID, imei, drv, nil, userID, nil, &action)
}

// Relay ---
func (sender *Sender) Relay(objID, imei, drv string, userID *string, idx uint16, on uint8, ton uint32, toff uint32) TerminalResponse {
	action := CommandAction{
		Id:    "relay",
		Index: uint32(idx),
		Act:   uint32(on),
		Ton:   ton,
		Toff:  toff,
	}
	return sender.Run(objID, imei, drv, nil, userID, nil, &action)
}

// State ---
func (sender *Sender) State(objID, imei, drv string, userID *string) TerminalResponse {
	action := CommandAction{
		Id: "state",
	}
	return sender.Run(objID, imei, drv, nil, userID, nil, &action)
}

// Reset ---
func (sender *Sender) Reset(objID, imei, drv string, userID *string) TerminalResponse {
	action := CommandAction{
		Id: "reset",
	}
	return sender.Run(objID, imei, drv, nil, userID, nil, &action)
}

// Auth - update auth token
func (sender *Sender) Auth(objID, imei, drv string, userID *string) TerminalResponse {
	action := CommandAction{
		Id:  "guard",
		Act: 100,
	}
	return sender.Run(objID, imei, drv, nil, userID, nil, &action)
}

// SetLogger - add log consumer function
func (sender *Sender) SetLogger(log func(string, string, *string, *string, *CommandAction, *TerminalResponse, error)) {
	sender.Loggers = append(sender.Loggers, log)
}

// SetBitErrors ---
func (response *TerminalResponse) SetBitErrors() {
	if response.Errors == nil {
		response.Errors = make([]CommandError, 0)
	}
	result := &response.Result
	//ignition
	if (*result & (1 << 0)) != 0 {
		response.Errors = append(response.Errors, CommandError{1100, ErrorCodes[1100]})
	}
	//parking
	if (*result & (1 << 1)) != 0 {
		response.Errors = append(response.Errors, CommandError{1101, ErrorCodes[1101]})
	}
	//doors
	if (*result & (1 << 2)) != 0 {
		response.Errors = append(response.Errors, CommandError{1102, ErrorCodes[1102]})
	}
	//trunk
	if (*result & (1 << 3)) != 0 {
		response.Errors = append(response.Errors, CommandError{1103, ErrorCodes[1103]})
	}
	//volume sensor
	if (*result & (1 << 4)) != 0 {
		response.Errors = append(response.Errors, CommandError{1104, ErrorCodes[1104]})
	}
	//hood
	if (*result & (1 << 5)) != 0 {
		response.Errors = append(response.Errors, CommandError{1105, ErrorCodes[1105]})
	}
	//lights
	if (*result & (1 << 6)) != 0 {
		response.Errors = append(response.Errors, CommandError{1106, ErrorCodes[1106]})
	}
	//already running
	if (*result & (1 << 7)) != 0 {
		response.Errors = append(response.Errors, CommandError{1107, ErrorCodes[1107]})
	}

	//already guard
	if (*result & guard_error_already) != 0 {
		//response.Errors = append(response.Errors, CommandError{1107, ErrorCodes[1107]})
	}
	//error sensor
	if (*result & guard_error_sensor) != 0 {
		response.Errors = append(response.Errors, CommandError{1108, ErrorCodes[1108]})
	}
	//error guard
	if (*result & guard_error_guard) != 0 {
		response.Errors = append(response.Errors, CommandError{1109, ErrorCodes[1109]})
	}
	//error can park
	if (*result & guard_error_can_park) != 0 {
		response.Errors = append(response.Errors, CommandError{1101, ErrorCodes[1101]})
	}

	//error can brake
	if (*result & guard_error_can_brake) != 0 {
		response.Errors = append(response.Errors, CommandError{1110, ErrorCodes[1110]})
	}

	//error timeout
	if (*result & guard_error_timeout) != 0 {
		response.Errors = append(response.Errors, CommandError{-3, ErrorCodes[-3]})
	}

	//error disabled
	if (*result & guard_error_disable) != 0 {
		response.Errors = append(response.Errors, CommandError{1111, ErrorCodes[1111]})
	}

	//error other
	if (*result & guard_error_other) != 0 {
		response.Errors = append(response.Errors, CommandError{1112, ErrorCodes[1112]})
	}

}

// GetErrorsText ---
func (response *TerminalResponse) GetErrorsText() string {
	res := "<unknown>"
	if len(response.Errors) == 0 {
		return res
	} else {
		strArray := make([]string, 0)
		for _, val := range response.Errors {
			strArray = append(strArray, val.Message)
		}
		return strings.Join(strArray, ",")
	}
}

// NewSender ---
func NewSender(url string) *Sender {
	return &Sender{
		http:        &http.Client{Timeout: 40000000000},
		terminalURL: &url,
	}
}
