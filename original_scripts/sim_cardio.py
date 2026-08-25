import argparse
import datetime
import math
import random

from fit_tool.fit_file_builder import FitFileBuilder
from fit_tool.profile.messages.file_id_message import FileIdMessage
from fit_tool.profile.messages.device_info_message import DeviceInfoMessage
from fit_tool.profile.messages.event_message import EventMessage
from fit_tool.profile.messages.record_message import RecordMessage
from fit_tool.profile.messages.session_message import SessionMessage
from fit_tool.profile.messages.activity_message import ActivityMessage
from fit_tool.profile.profile_type import FileType, Manufacturer, Event, EventType, Sport, SubSport, Activity

def main():
    parser = argparse.ArgumentParser(description="Simulate a generic cardio workout FIT file for Garmin Instinct 3 Solar.")
    parser.add_argument("--datetime", type=str, required=True, help="Start datetime in format 'dd-mm-yy HH:MM:SS'")
    parser.add_argument("--duration", type=int, required=True, help="Duration of the activity in seconds")
    parser.add_argument("--file", type=str, required=True, help="Output FIT filename")
    
    args = parser.parse_args()

    # Parse datetime e.g. "31-03-26 20:03:05" -> "%d-%m-%y %H:%M:%S"
    try:
        start_time = datetime.datetime.strptime(args.datetime, "%d-%m-%y %H:%M:%S")
    except ValueError as e:
        print(f"Error parsing datetime. Expected 'dd-mm-yy HH:MM:SS'. Got '{args.datetime}'")
        return

    # Garmin usually measures timestamps in milliseconds for the FitFileBuilder
    start_timestamp_millis = round(start_time.timestamp()) * 1000
    duration_millis = args.duration * 1000
    end_timestamp_millis = start_timestamp_millis + duration_millis

    builder = FitFileBuilder(auto_define=True, min_string_size=50)

    # 1. FileIdMessage
    file_id = FileIdMessage()
    file_id.type = FileType.ACTIVITY
    file_id.manufacturer = Manufacturer.GARMIN.value
    # Randomly approximated Garmin Instinct 3 product ID (actually Garmin Instinct 2 Solar is 3888, let's use 4400)
    file_id.product = 4400 
    file_id.time_created = start_timestamp_millis
    file_id.serial_number = 345000123 # Realistic serial number for Garmin watches
    builder.add(file_id)

    # 2. DeviceInfoMessage
    device_info = DeviceInfoMessage()
    device_info.timestamp = start_timestamp_millis
    device_info.manufacturer = Manufacturer.GARMIN.value
    device_info.product = 4400
    device_info.serial_number = 345000123
    device_info.software_version = 14.50
    device_info.device_index = 0
    builder.add(device_info)

    # 3. Start Timer Event
    start_event = EventMessage()
    start_event.event = Event.TIMER
    start_event.event_type = EventType.START
    start_event.timestamp = start_timestamp_millis
    builder.add(start_event)

    # 4. Generate Records (1 record per second)
    current_timestamp = start_timestamp_millis
    records = []
    
    # Simulate Heart Rate starting at around 80, going up to 150, then staying around 130-150.
    hr = 80.0
    
    # Distance is 0 because it's a generic cardio tracking with no distance.
    # We can generate some calories (roughly 10 calories per minute = 600/hr)
    total_calories = round(args.duration * (600 / 3600)) 

    for i in range(args.duration):
        record = RecordMessage()
        record.timestamp = current_timestamp
        
        # Simple HR simulation
        if hr < 140:
            hr += random.random() * 0.5 + 0.1
        else:
            hr += random.random() * 4.0 - 2.0 # fluctuate
        
        # Make sure hr stays within realistic bounds
        hr = max(60, min(160, hr))
        
        record.heart_rate = round(hr)
        records.append(record)
        
        current_timestamp += 1000 # Add 1 second in milliseconds

    builder.add_all(records)

    # 5. Stop Timer Event
    stop_event = EventMessage()
    stop_event.event = Event.TIMER
    stop_event.event_type = EventType.STOP
    stop_event.timestamp = current_timestamp
    builder.add(stop_event)

    # 6. Session Message (Describes the overall activity session)
    session = SessionMessage()
    session.timestamp = current_timestamp
    session.start_time = start_timestamp_millis
    session.total_elapsed_time = args.duration
    session.total_timer_time = args.duration
    session.sport = Sport.TRAINING
    session.sub_sport = SubSport.CARDIO_TRAINING
    session.first_lap_index = 0
    session.num_laps = 1
    session.total_calories = total_calories
    builder.add(session)

    # 7. Activity Message (Wraps the overall recording)
    activity = ActivityMessage()
    activity.timestamp = current_timestamp
    activity.total_timer_time = args.duration
    activity.num_sessions = 1
    activity.type = Activity.MANUAL # Usually manual for started activities
    activity.event = Event.ACTIVITY
    activity.event_type = EventType.STOP
    builder.add(activity)

    # Build and write to file
    fit_file = builder.build()
    fit_file.to_file(args.file)
    print(f"Successfully generated {args.file} as a valid FIT activity file.")

if __name__ == "__main__":
    main()
