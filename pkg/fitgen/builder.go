package fitgen

import (
	"encoding/binary"
	"io"
	"os"
	"time"

	"github.com/tormoder/fit"
)

// Builder helps construct a Garmin FIT activity file.
type Builder struct {
	file     *fit.File
	activity *fit.ActivityFile
}

// NewBuilder initializes a new FIT Activity builder.
func NewBuilder(startTime time.Time, product uint16, serial uint32) *Builder {
	f, _ := fit.NewFile(fit.FileTypeActivity, fit.NewHeader(fit.V20, true))
	
	// FileId
	f.FileId.Type = fit.FileTypeActivity
	f.FileId.Manufacturer = fit.ManufacturerGarmin
	f.FileId.Product = product
	f.FileId.TimeCreated = startTime
	f.FileId.SerialNumber = serial

	act, _ := f.Activity()

	return &Builder{
		file:     f,
		activity: act,
	}
}

// AddDeviceInfo adds a device info message.
func (b *Builder) AddDeviceInfo(timestamp time.Time, product uint16, serial uint32, swVersion float32) {
	msg := fit.NewDeviceInfoMsg()
	msg.Timestamp = timestamp
	msg.Manufacturer = fit.ManufacturerGarmin
	msg.Product = product
	msg.SerialNumber = serial
	msg.SoftwareVersion = uint16(swVersion * 100)
	msg.DeviceIndex = 0
	b.activity.DeviceInfos = append(b.activity.DeviceInfos, msg)
}

// AddEvent adds an event message.
func (b *Builder) AddEvent(timestamp time.Time, event fit.Event, eventType fit.EventType) {
	msg := fit.NewEventMsg()
	msg.Timestamp = timestamp
	msg.Event = event
	msg.EventType = eventType
	b.activity.Events = append(b.activity.Events, msg)
}

// AddRecord adds a record message.
func (b *Builder) AddRecord(record *fit.RecordMsg) {
	b.activity.Records = append(b.activity.Records, record)
}

// AddLap adds a lap message.
func (b *Builder) AddLap(lap *fit.LapMsg) {
	b.activity.Laps = append(b.activity.Laps, lap)
}

// AddSession adds a session message.
func (b *Builder) AddSession(session *fit.SessionMsg) {
	b.activity.Sessions = append(b.activity.Sessions, session)
}

// AddActivity wraps up with an activity message.
func (b *Builder) AddActivity(timestamp time.Time, totalTimerTime uint32, numSessions uint16, event fit.Event, eventType fit.EventType) {
	msg := fit.NewActivityMsg()
	msg.Timestamp = timestamp
	msg.TotalTimerTime = totalTimerTime
	msg.NumSessions = numSessions
	msg.Type = fit.ActivityModeManual
	msg.Event = event
	msg.EventType = eventType
	b.activity.Activity = msg
}

// WriteToFile writes the accumulated FIT file to disk.
func (b *Builder) WriteToFile(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	return b.WriteTo(file)
}

// WriteTo writes the accumulated FIT file to the provided io.Writer.
func (b *Builder) WriteTo(w io.Writer) error {
	return fit.Encode(w, b.file, binary.LittleEndian)
}
