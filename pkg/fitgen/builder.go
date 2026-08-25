package fitgen

import (
	"os"
	"time"

	"github.com/tormoder/fit"
)

// Builder helps construct a Garmin FIT activity file.
type Builder struct {
	activity *fit.ActivityFile
}

// NewBuilder initializes a new FIT Activity builder.
func NewBuilder(startTime time.Time, product uint16, serial uint32) *Builder {
	activity, _ := fit.NewActivityFile()
	
	// FileId
	activity.FileId.Type = fit.FileTypeActivity
	activity.FileId.Manufacturer = fit.ManufacturerGarmin
	activity.FileId.Product = product
	activity.FileId.TimeCreated = startTime
	activity.FileId.SerialNumber = serial

	return &Builder{
		activity: activity,
	}
}

// AddDeviceInfo adds a device info message.
func (b *Builder) AddDeviceInfo(timestamp time.Time, product uint16, serial uint32, swVersion float32) {
	msg := fit.NewDeviceInfoMsg()
	msg.Timestamp = timestamp
	msg.Manufacturer = fit.ManufacturerGarmin
	msg.Product = product
	msg.SerialNumber = uint32z(serial)
	msg.SoftwareVersion = swVersion
	msg.DeviceIndex = 0
	b.activity.DeviceInfoMsgs = append(b.activity.DeviceInfoMsgs, msg)
}

// uint32z handles optional uint32
func uint32z(v uint32) uint32 { return v }

// AddSport adds a sport message.
func (b *Builder) AddSport(sport fit.Sport, subSport fit.SubSport, name string) {
	msg := fit.NewSportMsg()
	msg.Sport = sport
	msg.SubSport = subSport
	msg.Name = name
	b.activity.SportMsgs = append(b.activity.SportMsgs, msg)
}

// AddEvent adds an event message.
func (b *Builder) AddEvent(timestamp time.Time, event fit.Event, eventType fit.EventType) {
	msg := fit.NewEventMsg()
	msg.Timestamp = timestamp
	msg.Event = event
	msg.EventType = eventType
	b.activity.EventMsgs = append(b.activity.EventMsgs, msg)
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
	msg.Type = fit.ActivityManual
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

	fitFile := b.activity.File()
	return fit.Encode(file, fitFile, fit.LittleEndian)
}
