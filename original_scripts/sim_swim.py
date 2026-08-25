import argparse
import datetime
import math
import random

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
    FileType,
    Manufacturer,
    Event,
    EventType,
    Sport,
    SubSport,
    Activity,
    LapTrigger,
    SessionTrigger,
    SwimStroke,
)

SWIM_STROKE_CHOICES = {
    'freestyle': SwimStroke.FREESTYLE,
    'breaststroke': SwimStroke.BREASTSTROKE,
    'butterfly': SwimStroke.BUTTERFLY,
    'backstroke': SwimStroke.BACKSTROKE,
    'drill': SwimStroke.DRILL,
    'im': SwimStroke.IM,
    'mixed': SwimStroke.MIXED,
}


def parse_args():
    parser = argparse.ArgumentParser(
        description="Simulate a swimming FIT file for Garmin Connect import."
    )
    parser.add_argument(
        "--datetime",
        type=str,
        required=True,
        help="Start datetime in format 'dd-mm-yy HH:MM:SS'",
    )
    parser.add_argument(
        "--distance",
        type=float,
        required=True,
        help="Total swim distance in kilometers (e.g. 1.0, 2.5)",
    )
    parser.add_argument(
        "--speed",
        type=float,
        required=True,
        help="Average swimming speed in km/h (e.g. 1.5 - 3.0)",
    )
    parser.add_argument(
        "--pool-length",
        type=int,
        default=25,
        help="Pool length in meters (default: 25)",
    )
    parser.add_argument(
        "--stroke",
        type=str,
        default="freestyle",
        choices=list(SWIM_STROKE_CHOICES.keys()),
        help="Swimming stroke style (default: freestyle)",
    )
    parser.add_argument(
        "--file",
        type=str,
        required=True,
        help="Output FIT filename",
    )
    return parser.parse_args()


def main():
    args = parse_args()

    try:
        start_time = datetime.datetime.strptime(args.datetime, "%d-%m-%y %H:%M:%S")
    except ValueError:
        print(f"Error parsing datetime. Expected 'dd-mm-yy HH:MM:SS'. Got '{args.datetime}'")
        return

    if args.distance <= 0 or args.speed <= 0:
        print("Distance and speed must both be positive values.")
        return

    distance_m = args.distance * 1000.0
    speed_m_s = args.speed * 1000.0 / 3600.0
    total_time_s = int(distance_m / speed_m_s)

    if total_time_s <= 0:
        print("Computed swim duration is zero. Check distance and speed values.")
        return

    pool_length_m = max(1, args.pool_length)
    swim_stroke = SWIM_STROKE_CHOICES[args.stroke]

    # Estimate stroke rate and calories
    stroke_rate_spm = max(40.0, min(72.0, 46.0 + (args.speed - 1.0) * 14.0))
    calories_per_hour = 320 + args.speed * 90
    total_calories = round(total_time_s * (calories_per_hour / 3600.0))

    estimated_total_cycles = total_time_s * stroke_rate_spm / 60.0
    if estimated_total_cycles <= 0:
        estimated_total_cycles = max(1.0, total_time_s / 2.0)

    cycle_length_m = distance_m / estimated_total_cycles
    cycle_length_m = max(1.2, min(3.5, cycle_length_m))

    pace_min_per_100m = (total_time_s / 60.0) / (distance_m / 100.0)
    print(f"Simulating swimming activity: {args.distance:.2f} km at {args.speed:.2f} km/h")
    print(f"Estimated duration: {total_time_s} seconds ({total_time_s/60:.2f} mins)")
    print(f"Pool length: {pool_length_m} m, stroke: {args.stroke}")
    print(f"Estimated pace: {int(pace_min_per_100m)}:{int((pace_min_per_100m % 1) * 60):02d} min/100m")
    print(f"Estimated calories: ~{total_calories} kcal")

    start_timestamp_ms = round(start_time.timestamp()) * 1000
    current_timestamp = start_timestamp_ms

    builder = FitFileBuilder(auto_define=True, min_string_size=50)

    file_id = FileIdMessage()
    file_id.type = FileType.ACTIVITY
    file_id.manufacturer = Manufacturer.GARMIN.value
    file_id.product = 4400
    file_id.time_created = start_timestamp_ms
    file_id.serial_number = 345000124
    builder.add(file_id)

    device_info = DeviceInfoMessage()
    device_info.timestamp = start_timestamp_ms
    device_info.manufacturer = Manufacturer.GARMIN.value
    device_info.product = 4400
    device_info.serial_number = 345000124
    device_info.software_version = 14.50
    device_info.device_index = 0
    builder.add(device_info)

    sport_msg = SportMessage()
    sport_msg.sport = Sport.SWIMMING
    sport_msg.sub_sport = SubSport.LAP_SWIMMING
    sport_msg.sport_name = "Swimming"
    builder.add(sport_msg)

    start_event = EventMessage()
    start_event.event = Event.TIMER
    start_event.event_type = EventType.START
    start_event.timestamp = start_timestamp_ms
    builder.add(start_event)

    RESTING_HR = random.uniform(60.0, 70.0)
    MAX_HR_CAP = 170
    hr = RESTING_HR
    target_hr = min(100.0 + (args.speed - 1.5) * 20.0, MAX_HR_CAP - 5)
    records = []
    all_hr_values = []
    stroke_cycles = 0.0
    total_stroke_counts = 0.0

    warmup_duration = min(180, total_time_s // 4)
    drift_per_second = 0.0025

    for t in range(total_time_s + 1):
        target_dist = min(distance_m, t * speed_m_s)

        instant_speed = speed_m_s * random.uniform(0.95, 1.05)
        record = RecordMessage()
        record.timestamp = current_timestamp
        record.distance = target_dist
        record.speed = instant_speed
        record.swim_stroke = swim_stroke

        if t < warmup_duration:
            phase = t / warmup_duration if warmup_duration > 0 else 1.0
            hr_target = RESTING_HR + (target_hr - RESTING_HR) * (1 - (1 - phase) ** 2)
        else:
            hr_target = target_hr + min((t - warmup_duration) * drift_per_second, 5.0)

        hr += (hr_target - hr) * random.uniform(0.05, 0.14)
        hr += random.uniform(-1.2, 1.2)
        hr = max(RESTING_HR, min(MAX_HR_CAP, hr))
        record.heart_rate = round(hr)
        all_hr_values.append(round(hr))

        stroke_cycles += stroke_rate_spm / 60.0 * random.uniform(0.96, 1.04)
        record.total_cycles = round(stroke_cycles)

        stroke_count = stroke_rate_spm + random.uniform(-3.5, 3.5)
        record.stroke_count = round(max(30.0, min(80.0, stroke_count)))
        total_stroke_counts += record.stroke_count / 60.0

        record.cycle_length = round(cycle_length_m, 2)
        records.append(record)
        current_timestamp += 1000

    builder.add_all(records)

    stop_event = EventMessage()
    stop_event.event = Event.TIMER
    stop_event.event_type = EventType.STOP
    stop_event.timestamp = current_timestamp
    builder.add(stop_event)

    avg_hr = round(sum(all_hr_values) / len(all_hr_values)) if all_hr_values else 0
    max_hr = max(all_hr_values) if all_hr_values else 0
    min_hr = min(all_hr_values) if all_hr_values else 0
    total_cycles_int = round(stroke_cycles)
    avg_stroke_count = round(stroke_rate_spm)

    lap = LapMessage()
    lap.timestamp = current_timestamp
    lap.start_time = start_timestamp_ms
    lap.total_elapsed_time = total_time_s
    lap.total_timer_time = total_time_s
    lap.total_distance = distance_m
    lap.total_calories = total_calories
    lap.total_cycles = total_cycles_int
    lap.total_strokes = total_cycles_int
    lap.avg_stroke_count = avg_stroke_count
    lap.avg_stroke_distance = round(cycle_length_m, 2)
    lap.swim_stroke = swim_stroke
    lap.pool_length = pool_length_m
    lap.sport = Sport.SWIMMING
    lap.sub_sport = SubSport.LAP_SWIMMING
    lap.lap_trigger = LapTrigger.SESSION_END
    builder.add(lap)

    session = SessionMessage()
    session.timestamp = current_timestamp
    session.start_time = start_timestamp_ms
    session.total_elapsed_time = total_time_s
    session.total_timer_time = total_time_s
    session.sport = Sport.SWIMMING
    session.sub_sport = SubSport.LAP_SWIMMING
    session.first_lap_index = 0
    session.num_laps = 1
    session.total_distance = distance_m
    session.total_calories = total_calories
    session.total_cycles = total_cycles_int
    session.total_strokes = total_cycles_int
    session.avg_stroke_count = avg_stroke_count
    session.avg_stroke_distance = round(cycle_length_m, 2)
    session.swim_stroke = swim_stroke
    session.pool_length = pool_length_m
    session.trigger = SessionTrigger.ACTIVITY_END
    builder.add(session)

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
    print(f"Generated swim FIT: {args.file}")
    print(f"Duration: {hours:02d}:{minutes:02d}:{seconds:02d}")
    print(f"Distance: {distance_m:.1f} m, pace: {pace_min_per_100m:.2f} min/100m")
    print(f"Calories: ~{total_calories} kcal")
    print(f"Heart Rate: avg {avg_hr} / max {max_hr} / min {min_hr} bpm")
    print(f"Total cycles: {total_cycles_int}")


if __name__ == "__main__":
    main()
