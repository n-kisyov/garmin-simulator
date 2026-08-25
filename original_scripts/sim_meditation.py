import argparse
import datetime
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
    FileType, Manufacturer, Event, EventType, Sport, SubSport, Activity,
    LapTrigger, SessionTrigger
)


def main():
    parser = argparse.ArgumentParser(
        description="Simulate a meditation FIT file for Garmin devices.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""Examples:
  10-minute meditation:
    python sim_meditation.py --datetime "15-04-26 07:00:00" --duration 600 --file meditation.fit

  20-minute morning meditation:
    python sim_meditation.py --datetime "15-04-26 06:30:00" --duration 1200 --file meditation.fit
"""
    )
    parser.add_argument("--datetime", type=str, required=True,
                        help="Start datetime in format 'dd-mm-yy HH:MM:SS'")
    parser.add_argument("--duration", type=int, required=True,
                        help="Duration of meditation in seconds (e.g. 600 = 10 min)")
    parser.add_argument("--file", type=str, required=True,
                        help="Output FIT filename")

    args = parser.parse_args()

    try:
        start_time = datetime.datetime.strptime(args.datetime, "%d-%m-%y %H:%M:%S")
    except ValueError:
        print(f"Error parsing datetime. Expected 'dd-mm-yy HH:MM:SS'. Got '{args.datetime}'")
        return

    duration_s = args.duration
    start_timestamp_ms = round(start_time.timestamp()) * 1000
    current_ts = start_timestamp_ms

    builder = FitFileBuilder(auto_define=True, min_string_size=50)

    # --- 1. File ID ---
    file_id = FileIdMessage()
    file_id.type = FileType.ACTIVITY
    file_id.manufacturer = Manufacturer.GARMIN.value
    file_id.product = 4400
    file_id.time_created = start_timestamp_ms
    file_id.serial_number = 345000124
    builder.add(file_id)

    # --- 2. Device Info ---
    device_info = DeviceInfoMessage()
    device_info.timestamp = start_timestamp_ms
    device_info.manufacturer = Manufacturer.GARMIN.value
    device_info.product = 4400
    device_info.serial_number = 345000124
    device_info.software_version = 14.50
    device_info.device_index = 0
    builder.add(device_info)

    # --- 3. Sport Message ---
    sport_msg = SportMessage()
    sport_msg.sport = Sport.TRAINING
    sport_msg.sub_sport = SubSport.GENERIC
    sport_msg.sport_name = "Meditation"
    builder.add(sport_msg)

    # --- 4. Timer Start ---
    start_event = EventMessage()
    start_event.event = Event.TIMER
    start_event.event_type = EventType.START
    start_event.timestamp = start_timestamp_ms
    builder.add(start_event)

    # --- 5. Generate Records ---
    #
    # Meditation HR model:
    #   - Initial HR ~68-75 bpm (seated, settling in)
    #   - First 1-2 minutes: HR gradually drops as breathing slows
    #   - Deep meditation: HR settles to 55-65 bpm
    #   - Very gentle oscillation (breathing rhythm, ~4-6 breaths/min)
    #   - Occasional micro-fluctuations from thoughts/shifts
    #   - Last minute: slight rise as session ends
    #
    INITIAL_HR = random.uniform(68, 75)
    DEEP_HR = random.uniform(55, 63)
    hr = INITIAL_HR
    records = []
    all_hr_values = []

    # Settling phase: first ~120 seconds
    settle_duration = min(120, duration_s // 4)
    # Emergence phase: last ~60 seconds
    emerge_start = max(duration_s - 60, duration_s * 3 // 4)

    # Breathing cycle: 4-6 breaths per minute -> period of 10-15 seconds
    breath_period = random.uniform(10.0, 15.0)

    # Calories: ~1.0-1.5 kcal/min for seated meditation
    cal_per_min = random.uniform(1.0, 1.5)
    total_calories = round(duration_s / 60.0 * cal_per_min)

    for t in range(duration_s):
        record = RecordMessage()
        record.timestamp = current_ts

        if t < settle_duration:
            # Settling: HR drifts down from initial to deep
            progress = t / settle_duration
            target_hr = INITIAL_HR + (DEEP_HR - INITIAL_HR) * (1 - (1 - progress) ** 2)
        elif t >= emerge_start:
            # Emergence: HR gently rises back toward initial
            emerge_progress = (t - emerge_start) / (duration_s - emerge_start)
            emerge_target = DEEP_HR + (INITIAL_HR - DEEP_HR) * 0.4  # doesn't fully return
            target_hr = DEEP_HR + (emerge_target - DEEP_HR) * emerge_progress
        else:
            # Deep meditation: stable near DEEP_HR
            target_hr = DEEP_HR

        # Breathing rhythm: sinusoidal oscillation ±1.5 bpm
        breath_phase = (t / breath_period) * 2 * 3.14159
        breath_modulation = 1.5 * (0.5 * (1 + __import__('math').sin(breath_phase)))

        # Very slow convergence for smooth, calm HR
        hr += (target_hr + breath_modulation - hr) * random.uniform(0.02, 0.06)

        # Minimal jitter (meditation HR is very stable)
        hr += random.uniform(-0.3, 0.3)

        # Occasional micro-spike (shifting position, stray thought) ~2% chance
        if random.random() < 0.02:
            hr += random.uniform(1, 3)

        hr = max(48, min(85, hr))

        record.heart_rate = round(hr)
        all_hr_values.append(round(hr))
        records.append(record)
        current_ts += 1000

    builder.add_all(records)

    # --- 6. Timer Stop ---
    stop_event = EventMessage()
    stop_event.event = Event.TIMER
    stop_event.event_type = EventType.STOP
    stop_event.timestamp = current_ts
    builder.add(stop_event)

    # Compute HR stats
    avg_hr = round(sum(all_hr_values) / len(all_hr_values)) if all_hr_values else 0
    max_hr = max(all_hr_values) if all_hr_values else 0
    min_hr = min(all_hr_values) if all_hr_values else 0

    # --- 7. Lap ---
    lap = LapMessage()
    lap.timestamp = current_ts
    lap.start_time = start_timestamp_ms
    lap.total_elapsed_time = duration_s
    lap.total_timer_time = duration_s
    lap.total_calories = total_calories
    lap.avg_heart_rate = avg_hr
    lap.max_heart_rate = max_hr
    lap.sport = Sport.TRAINING
    lap.sub_sport = SubSport.GENERIC
    lap.lap_trigger = LapTrigger.SESSION_END
    builder.add(lap)

    # --- 8. Session ---
    session = SessionMessage()
    session.timestamp = current_ts
    session.start_time = start_timestamp_ms
    session.total_elapsed_time = duration_s
    session.total_timer_time = duration_s
    session.sport = Sport.TRAINING
    session.sub_sport = SubSport.GENERIC
    session.first_lap_index = 0
    session.num_laps = 1
    session.total_calories = total_calories
    session.avg_heart_rate = avg_hr
    session.max_heart_rate = max_hr
    session.trigger = SessionTrigger.ACTIVITY_END
    builder.add(session)

    # --- 9. Activity ---
    activity = ActivityMessage()
    activity.timestamp = current_ts
    activity.total_timer_time = duration_s
    activity.num_sessions = 1
    activity.type = Activity.MANUAL
    activity.event = Event.ACTIVITY
    activity.event_type = EventType.STOP
    builder.add(activity)

    # Build and write
    fit_file = builder.build()
    fit_file.to_file(args.file)

    hours = duration_s // 3600
    minutes = (duration_s % 3600) // 60
    seconds = duration_s % 60
    print(f"Meditation summary:")
    print(f"  Duration:   {hours:02d}:{minutes:02d}:{seconds:02d}")
    print(f"  Calories:   ~{total_calories} kcal")
    print(f"  Heart Rate: avg {avg_hr} / max {max_hr} / min {min_hr} bpm")
    print(f"Successfully generated {args.file}")


if __name__ == "__main__":
    main()
