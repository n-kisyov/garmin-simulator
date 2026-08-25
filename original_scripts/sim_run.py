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
    parser = argparse.ArgumentParser(description="Simulate a running FIT file using a KML route.")
    parser.add_argument("--datetime", type=str, required=True, help="Start datetime in format 'dd-mm-yy HH:MM:SS'")
    parser.add_argument("--speed", type=float, required=True, help="Average running speed in km/h (e.g. 8.0 - 12.0)")
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

    # Calculate duration based on speed
    speed_m_s = args.speed * 1000.0 / 3600.0
    total_time_s = int(total_dist_m / speed_m_s)

    if total_time_s <= 0:
        print("Total route distance or speed is too small, duration=0.")
        return

    # Pace in min/km
    pace_min_per_km = (total_time_s / 60.0) / (total_dist_m / 1000.0)

    print(f"Route parsed. Total distance: {total_dist_m:.1f} meters ({total_dist_m/1000:.2f} km).")
    print(f"Average speed: {args.speed} km/h  |  Pace: {int(pace_min_per_km)}:{int((pace_min_per_km % 1) * 60):02d} min/km")
    print(f"Calculated duration: {total_time_s} seconds ({total_time_s/60:.2f} mins).")

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
    sport_msg.sport = Sport.RUNNING
    sport_msg.sub_sport = SubSport.GENERIC
    sport_msg.sport_name = "Running"
    builder.add(sport_msg)

    # 4. Start Timer Event
    start_event = EventMessage()
    start_event.event = Event.TIMER
    start_event.event_type = EventType.START
    start_event.timestamp = start_timestamp_millis
    builder.add(start_event)

    # 5. Generate Records (1 record per second)
    #
    # Realistic HR model for running:
    #   - Starts around 75-85 bpm, ramps up in the first ~3 minutes
    #   - Settles into a steady zone of 145-175 bpm depending on pace
    #   - Natural variation ±2-3 bpm beat-to-beat
    #   - Hard cap at 185 bpm
    #
    RESTING_HR = random.uniform(72, 82)
    MAX_HR_CAP = 170
    hr = RESTING_HR
    records = []
    all_hr_values = []

    # Running cadence: ~160-180 steps per minute
    cadence_base = 170.0
    total_steps = 0

    # Track last recorded position for lap end_position
    last_lat, last_lon = start_lat, start_lon

    # Steady-state HR target based on speed (faster = higher HR)
    # 8 km/h → ~145, 10 km/h → ~155, 12 km/h → ~168, 14 km/h → ~178
    steady_hr = min(110 + args.speed * 5.5, MAX_HR_CAP - 5)

    # Calories: ~700-1000 kcal/hr for running depending on speed
    cal_per_hour = 600 + args.speed * 30
    total_calories = round(total_time_s * (cal_per_hour / 3600))

    # Warm-up phase: first 180 seconds HR ramps up
    warmup_duration = min(180, total_time_s // 3)

    pt_idx = 0

    for t in range(total_time_s + 1):
        target_dist = t * speed_m_s

        # Advance point index
        while pt_idx < len(points) - 2 and dists[pt_idx + 1] < target_dist:
            pt_idx += 1

        # Interpolate position
        d_start = dists[pt_idx]
        d_end = dists[pt_idx + 1]

        fraction = 0.0
        if d_end > d_start:
            fraction = (target_dist - d_start) / (d_end - d_start)
            fraction = max(0.0, min(1.0, fraction))

        lon = points[pt_idx][0] + fraction * (points[pt_idx+1][0] - points[pt_idx][0])
        lat = points[pt_idx][1] + fraction * (points[pt_idx+1][1] - points[pt_idx][1])

        last_lat, last_lon = lat, lon

        # Speed variation: ±5% natural fluctuation
        instant_speed = speed_m_s * random.uniform(0.95, 1.05)

        record = RecordMessage()
        record.timestamp = current_timestamp
        record.position_lat = lat
        record.position_long = lon
        record.distance = target_dist
        record.speed = instant_speed

        # HR simulation
        if t < warmup_duration:
            # Warm-up: ramp from resting toward steady state
            warmup_progress = t / warmup_duration
            target_hr = RESTING_HR + (steady_hr - RESTING_HR) * (1 - (1 - warmup_progress) ** 2)
        else:
            # Steady state with slight drift upward (cardiac drift)
            elapsed_after_warmup = t - warmup_duration
            drift = min(elapsed_after_warmup * 0.003, 8)  # up to +8 bpm over long runs
            target_hr = steady_hr + drift

        # Smooth convergence + jitter
        hr += (target_hr - hr) * random.uniform(0.05, 0.15)
        hr += random.uniform(-1.5, 1.5)
        hr = max(RESTING_HR, min(MAX_HR_CAP, hr))
        record.heart_rate = round(hr)
        all_hr_values.append(round(hr))

        # Cadence with natural variation (±5 spm)
        cadence = cadence_base + random.uniform(-5, 5)
        cadence = max(155, min(190, cadence))
        record.cadence = round(cadence)
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

    total_strides = round(total_steps / 2)

    # Compute HR stats
    avg_hr = round(sum(all_hr_values) / len(all_hr_values)) if all_hr_values else 0
    max_hr = max(all_hr_values) if all_hr_values else 0
    min_hr = min(all_hr_values) if all_hr_values else 0

    # 7. Lap Message — use actual last recorded GPS point as end position
    lap = LapMessage()
    lap.timestamp = current_timestamp
    lap.start_time = start_timestamp_millis
    lap.total_elapsed_time = total_time_s
    lap.total_timer_time = total_time_s
    lap.total_distance = total_dist_m
    lap.total_calories = total_calories
    lap.total_strides = total_strides
    lap.avg_heart_rate = avg_hr
    lap.max_heart_rate = max_hr
    lap.start_position_lat = start_lat
    lap.start_position_long = start_lon
    lap.end_position_lat = last_lat
    lap.end_position_long = last_lon
    lap.sport = Sport.RUNNING
    lap.sub_sport = SubSport.GENERIC
    lap.lap_trigger = LapTrigger.SESSION_END
    builder.add(lap)

    # 8. Session Message — bounding box from ALL route points
    session = SessionMessage()
    session.timestamp = current_timestamp
    session.start_time = start_timestamp_millis
    session.total_elapsed_time = total_time_s
    session.total_timer_time = total_time_s
    session.sport = Sport.RUNNING
    session.sub_sport = SubSport.GENERIC
    session.first_lap_index = 0
    session.num_laps = 1
    session.total_distance = total_dist_m
    session.total_calories = total_calories
    session.total_strides = total_strides
    session.avg_heart_rate = avg_hr
    session.max_heart_rate = max_hr
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
    print(f"Total strides: {total_strides}")
    print(f"Calories: ~{total_calories} kcal")
    print(f"Heart Rate: avg {avg_hr} / max {max_hr} / min {min_hr} bpm")
    print(f"Duration: {hours:02d}:{minutes:02d}:{seconds:02d}")
    print(f"Successfully generated {args.file} as a valid FIT running activity file.")

if __name__ == "__main__":
    main()
