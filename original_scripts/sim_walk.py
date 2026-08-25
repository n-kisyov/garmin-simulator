import argparse
import datetime
import math
import random
import re

from fit_tool.fit_file_builder import FitFileBuilder
from fit_tool.profile.messages.file_id_message import FileIdMessage
from fit_tool.profile.messages.device_info_message import DeviceInfoMessage
from fit_tool.profile.messages.sport_message import SportMessage
from fit_tool.profile.messages.event_message import EventMessage
from fit_tool.profile.messages.record_message import RecordMessage
from fit_tool.profile.messages.lap_message import LapMessage
from fit_tool.profile.messages.session_message import SessionMessage
from fit_tool.profile.messages.activity_message import ActivityMessage
from fit_tool.profile.profile_type import (
    FileType, Manufacturer, Event, EventType, Sport, SubSport, Activity,
    LapTrigger, SessionTrigger
)

def haversine(lon1, lat1, lon2, lat2):
    R = 6371e3  # Radius of earth in meters
    phi1, phi2 = math.radians(lat1), math.radians(lat2)
    dphi = math.radians(lat2 - lat1)
    dlambda = math.radians(lon2 - lon1)
    a = math.sin(dphi/2)**2 + math.cos(phi1)*math.cos(phi2)*math.sin(dlambda/2)**2
    return 2 * R * math.atan2(math.sqrt(a), math.sqrt(1-a))

def parse_kml_coordinates(kml_file) -> list[tuple[float, float]]:
    with open(kml_file, 'r', encoding='utf-8') as f:
        content = f.read()

    # Find the <coordinates> blocks. Ignore namespaces.
    matches = re.findall(r'<coordinates>(.*?)</coordinates>', content, re.DOTALL)
    if not matches:
        raise ValueError("Could not find any <coordinates> inside the KML file.")

    points = []
    for match in matches:
        chunks = match.strip().split()
        for chunk in chunks:
            parts = chunk.split(',')
            if len(parts) >= 2:
                lon = float(parts[0])
                lat = float(parts[1])
                points.append((lon, lat))
    
    return points

def main():
    parser = argparse.ArgumentParser(description="Simulate a walk FIT file using a KML route.")
    parser.add_argument("--datetime", type=str, required=True, help="Start datetime in format 'dd-mm-yy HH:MM:SS'")
    parser.add_argument("--speed", type=float, required=True, help="Average walking speed in km/h")
    parser.add_argument("--kml", type=str, required=True, help="Input KML file containing the route")
    parser.add_argument("--file", type=str, required=True, help="Output FIT filename")
    
    args = parser.parse_args()

    try:
        start_time = datetime.datetime.strptime(args.datetime, "%d-%m-%y %H:%M:%S")
    except ValueError:
        print(f"Error parsing datetime. Expected 'dd-mm-yy HH:MM:SS'. Got '{args.datetime}'")
        return

    points = parse_kml_coordinates(args.kml)
    if len(points) < 2:
        print("Not enough points found in the KML file to define a route.")
        return

    # Calculate cumulative distances
    dists = [0.0]
    for i in range(1, len(points)):
        d = haversine(points[i-1][0], points[i-1][1], points[i][0], points[i][1])
        dists.append(dists[-1] + d)

    total_dist_m = dists[-1]
    
    # Calculate duration
    speed_m_s = args.speed * 1000.0 / 3600.0
    total_time_s = int(total_dist_m / speed_m_s)
    
    if total_time_s <= 0:
        print("Total route distance or speed is too small duration=0.")
        return

    print(f"Route parsed. Total distance: {total_dist_m:.1f} meters.")
    print(f"Calculated Duration based on speed {args.speed} km/h: {total_time_s} seconds ({total_time_s/60:.2f} mins).")

    start_timestamp_millis = round(start_time.timestamp()) * 1000
    current_timestamp = start_timestamp_millis

    # Compute bounding box from ALL route points (not just start/end)
    all_lats = [p[1] for p in points]
    all_lons = [p[0] for p in points]
    bbox_min_lat = min(all_lats)
    bbox_max_lat = max(all_lats)
    bbox_min_lon = min(all_lons)
    bbox_max_lon = max(all_lons)

    # First coordinate for start position
    start_lat, start_lon = points[0][1], points[0][0]

    builder = FitFileBuilder(auto_define=True, min_string_size=50)

    # 1. FileIdMessage
    file_id = FileIdMessage()
    file_id.type = FileType.ACTIVITY
    file_id.manufacturer = Manufacturer.GARMIN.value
    file_id.product = 4400 
    file_id.time_created = start_timestamp_millis
    file_id.serial_number = 345000124 
    builder.add(file_id)

    # 2. DeviceInfoMessage
    device_info = DeviceInfoMessage()
    device_info.timestamp = start_timestamp_millis
    device_info.manufacturer = Manufacturer.GARMIN.value
    device_info.product = 4400
    device_info.serial_number = 345000124
    device_info.software_version = 14.50
    device_info.device_index = 0
    builder.add(device_info)

    # 3. Sport Message
    sport_msg = SportMessage()
    sport_msg.sport = Sport.WALKING
    sport_msg.sub_sport = SubSport.GENERIC
    sport_msg.sport_name = "Walking"
    builder.add(sport_msg)

    # 4. Start Timer Event
    start_event = EventMessage()
    start_event.event = Event.TIMER
    start_event.event_type = EventType.START
    start_event.timestamp = start_timestamp_millis
    builder.add(start_event)

    # 5. Generate Records (1 record per second)
    records = []
    hr = 80.0
    total_calories = round(total_time_s * (400 / 3600)) # roughly 400 kcal/hr for walking

    # Walking cadence: ~110-130 steps per minute is typical
    cadence_base = 120.0  # steps per minute
    total_steps = 0

    # Track last recorded position for lap end_position
    last_lat, last_lon = start_lat, start_lon

    # Iterator index for KML points
    pt_idx = 0 

    for t in range(total_time_s + 1):
        target_dist = t * speed_m_s

        # Advance point index until target_dist falls between pt_idx and pt_idx+1
        while pt_idx < len(points) - 2 and dists[pt_idx + 1] < target_dist:
            pt_idx += 1

        # Interpolate between pt_idx and pt_idx+1
        d_start = dists[pt_idx]
        d_end = dists[pt_idx + 1]
        
        fraction = 0.0
        if d_end > d_start:
            fraction = (target_dist - d_start) / (d_end - d_start)
            fraction = max(0.0, min(1.0, fraction))

        lon = points[pt_idx][0] + fraction * (points[pt_idx+1][0] - points[pt_idx][0])
        lat = points[pt_idx][1] + fraction * (points[pt_idx+1][1] - points[pt_idx][1])

        last_lat, last_lon = lat, lon

        record = RecordMessage()
        record.timestamp = current_timestamp
        record.position_lat = lat
        record.position_long = lon
        record.distance = target_dist
        record.speed = speed_m_s

        # Heart rate simulation: staying below 155
        if hr < 110:
            hr += random.random() * 0.5 + 0.1
        else:
            hr += random.random() * 4.0 - 2.0 
        
        hr = max(60, min(150, hr)) # max 150 < 155
        record.heart_rate = round(hr)

        # Cadence (steps per minute) with natural variation
        cadence = cadence_base + random.uniform(-5, 5)
        cadence = max(100, min(135, cadence))
        record.cadence = round(cadence)

        # Accumulate total steps (cadence is steps/min, we sample every 1 second)
        total_steps += cadence / 60.0

        records.append(record)
        current_timestamp += 1000

    builder.add_all(records)

    # 6. Stop Timer Event
    stop_event = EventMessage()
    stop_event.event = Event.TIMER
    stop_event.event_type = EventType.STOP
    stop_event.timestamp = current_timestamp
    builder.add(stop_event)

    total_strides = round(total_steps / 2)  # 1 stride = 2 steps

    # 7. Lap Message — use actual last recorded GPS point as end position
    lap = LapMessage()
    lap.timestamp = current_timestamp
    lap.start_time = start_timestamp_millis
    lap.total_elapsed_time = total_time_s
    lap.total_timer_time = total_time_s
    lap.total_distance = total_dist_m
    lap.total_calories = total_calories
    lap.total_strides = total_strides
    lap.start_position_lat = start_lat
    lap.start_position_long = start_lon
    lap.end_position_lat = last_lat
    lap.end_position_long = last_lon
    lap.sport = Sport.WALKING
    lap.sub_sport = SubSport.GENERIC
    lap.lap_trigger = LapTrigger.SESSION_END
    builder.add(lap)

    # 8. Session Message — bounding box from ALL interpolated route points
    session = SessionMessage()
    session.timestamp = current_timestamp
    session.start_time = start_timestamp_millis
    session.total_elapsed_time = total_time_s
    session.total_timer_time = total_time_s
    session.sport = Sport.WALKING
    session.sub_sport = SubSport.GENERIC
    session.first_lap_index = 0
    session.num_laps = 1
    session.total_distance = total_dist_m
    session.total_calories = total_calories
    session.total_strides = total_strides
    session.start_position_lat = start_lat
    session.start_position_long = start_lon
    session.nec_lat = bbox_max_lat
    session.nec_long = bbox_max_lon
    session.swc_lat = bbox_min_lat
    session.swc_long = bbox_min_lon
    session.trigger = SessionTrigger.ACTIVITY_END
    builder.add(session)

    # 9. Activity Message
    activity = ActivityMessage()
    activity.timestamp = current_timestamp
    activity.total_timer_time = total_time_s
    activity.num_sessions = 1
    activity.type = Activity.MANUAL
    activity.event = Event.ACTIVITY
    activity.event_type = EventType.STOP
    builder.add(activity)

    fit_file = builder.build()
    fit_file.to_file(args.file)

    hours = total_time_s // 3600
    minutes = (total_time_s % 3600) // 60
    seconds = total_time_s % 60
    print(f"Total steps: {round(total_steps)}")
    print(f"Duration: {hours:02d}:{minutes:02d}:{seconds:02d}")
    print(f"Successfully generated {args.file} as a valid FIT walk activity file.")

if __name__ == "__main__":
    main()
