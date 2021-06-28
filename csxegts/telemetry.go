package csxegts

import (
	"time"

	"github.com/kuznetsovin/egts-protocol/app/egts"

	"gitlab.com/battler/modules/csxtelemetry"
)

// EGTS_SR_STATE_DATA
func newStateData(pos *csxtelemetry.FlatPosition) *egts.SrStateData {
	data := egts.SrStateData{
		State:                  StActive,
		MainPowerSourceVoltage: uint8(pos.P[1021]),
		BackUpBatteryVoltage:   0,
		InternalBatteryVoltage: 0,
		IBU:                    "0",
		BBU:                    "0",
	}
	if pos.P[1101] != 0.0 && pos.P[1102] != 0.0 {
		data.NMS = "1"
	} else {
		data.NMS = "0"
	}

	return &data
}

func newPosData(pos *csxtelemetry.FlatPosition, rented bool) *egts.SrPosData {
	data := egts.SrPosData{
		NavigationTime: time.Unix(int64(pos.Time/1000), 0),
		Latitude:       pos.P[1101],
		Longitude:      pos.P[1102],
		BB:             "0",
		CS:             CsWGS84,
		FIX:            Fix2D,
		VLD:            "1",
		Direction:      byte(pos.P[1104]),
		Odometer:       float64ToByteArr(pos.P[1201], 3),
		Source:         SrcTimerEnabledIgnition,
	}
	data.DirectionHighestBit = data.Direction & 128
	if rented {
		data.DigitalInputs |= 128
	}
	if data.Latitude > 0 {
		data.LAHS = "0" // northern latitude
	} else {
		data.LAHS = "1" // south latitude
	}
	if data.Longitude > 0 {
		data.LOHS = "0" // eastern longitude
	} else {
		data.LOHS = "1" // west longitude
	}

	if pos.P[1103] != 0.0 {
		data.ALTE = "1"
		data.Altitude = float64ToByteArr(pos.P[1103], 3)

		if pos.P[1103] > 0.0 {
			data.AltitudeSign = 0
		} else {
			data.AltitudeSign = 1
		}
	} else {
		data.ALTE = "0"
	}

	if pos.P[1105] != 0.0 {
		data.MV = "1"
		data.Speed = uint16(pos.P[1105])
	} else {
		data.MV = "0"
	}

	return &data
}

func CreateTelemetryPacket(objectIdentifier uint32, pos *csxtelemetry.FlatPosition, rented bool) (*Packet, uint16) {
	recordData := []subrecordData{
		{SubrecordType: egts.SrPosDataType, SubrecordData: newPosData(pos, rented)},
		{SubrecordType: egts.SrStateDataType, SubrecordData: newStateData(pos)},
	}
	telemetryFrameData, recNum := newServiceFrameData(&objectIdentifier, RpPriorityNormal, egts.TeledataService, recordData)

	return newPacket(PacketIDCounter.Next(), egts.PtAppdataPacket, PacketPriorityNormal, telemetryFrameData), recNum
}