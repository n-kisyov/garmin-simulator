import argparse
import datetime
import random

from fit_tool.fit_file_builder import FitFileBuilder
from fit_tool.profile.messages.file_id_message import FileIdMessage
from fit_tool.profile.messages.device_info_message import DeviceInfoMessage
from fit_tool.profile.messages.event_message import EventMessage
from fit_tool.profile.messages.record_message import RecordMessage
from fit_tool.profile.messages.set_message import SetMessage
from fit_tool.profile.messages.exercise_title_message import ExerciseTitleMessage
from fit_tool.profile.messages.lap_message import LapMessage
from fit_tool.profile.messages.session_message import SessionMessage
from fit_tool.profile.messages.activity_message import ActivityMessage
from fit_tool.profile.profile_type import (
    FileType, Manufacturer, Event, EventType, Sport, SubSport, Activity,
    LapTrigger, SessionTrigger, SetType, ExerciseCategory
)

# Yoga poses and flows mapped to the closest FIT exercise categories.
# (display_name, ExerciseCategory, exercise_name_id, hold_time_seconds, kcal_per_min, is_meditation)
# For yoga, we use hold times instead of reps. Each "set" is one pose hold.
YOGA_POSES = [
    # Active poses
    ("Plank Hold",          ExerciseCategory.PLANK,          0,  60, 4.0, False),
    ("Warrior Pose",        ExerciseCategory.LUNGE,         32,  45, 3.0, False),
    ("Chair Pose",          ExerciseCategory.SQUAT,         61,  40, 3.5, False),
    ("Bridge Pose",         ExerciseCategory.HIP_RAISE,      0,  45, 2.5, False),
    ("Core Hold",           ExerciseCategory.CORE,           0,  50, 4.0, False),
    ("Cobra Stretch",       ExerciseCategory.HYPEREXTENSION, 0,  40, 2.0, False),
    ("Tree Pose",           ExerciseCategory.HIP_STABILITY, 44,  45, 2.0, False),
    ("Downward Dog",        ExerciseCategory.TOTAL_BODY,     0,  50, 2.5, False),
    # Meditation / relaxation
    ("Meditation",          ExerciseCategory.TOTAL_BODY,     0, 300, 1.2, True),
]


def main():
    parser = argparse.ArgumentParser(
        description="Simulate a yoga session FIT file for Garmin devices. "
                    "Rotates through yoga poses with a meditation segment.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""Examples:
  Default 8-pose session + meditation at the end:
    python sim_yoga.py --datetime "03-04-26 07:00:00" --file yoga.fit

  Longer session with 16 pose sets, 2 meditation blocks, custom transition rest:
    python sim_yoga.py --datetime "03-04-26 07:00:00" --poses 16 --meditation 2 --rest 15 --file yoga.fit
"""
    )
    parser.add_argument("--datetime", type=str, required=True,
                        help="Start datetime in format 'dd-mm-yy HH:MM:SS'")
    parser.add_argument("--file", type=str, required=True,
                        help="Output FIT filename")
    parser.add_argument("--poses", type=int, default=8,
                        help="Number of active pose sets (default: 8 = one full rotation)")
    parser.add_argument("--meditation", type=int, default=1,
                        help="Number of meditation blocks to include (default: 1, placed at end)")
    parser.add_argument("--meditation-duration", type=int, default=300,
                        help="Duration of each meditation block in seconds (default: 300 = 5 min)")
    parser.add_argument("--rest", type=int, default=10,
                        help="Transition time between poses in seconds (default: 10)")

    args = parser.parse_args()

    try:
        start_time = datetime.datetime.strptime(args.datetime, "%d-%m-%y %H:%M:%S")
    except ValueError:
        print(f"Error parsing datetime. Expected 'dd-mm-yy HH:MM:SS'. Got '{args.datetime}'")
        return

    num_poses = args.poses
    num_meditations = args.meditation
    meditation_duration = args.meditation_duration
    rest_seconds = args.rest

    # Build schedule: rotate through active poses, then add meditation blocks
    active_poses = [p for p in YOGA_POSES if not p[5]]
    schedule = []
    for i in range(num_poses):
        pose = active_poses[i % len(active_poses)]
        schedule.append(pose)
    # Add meditation blocks at end
    meditation_entry = ("Meditation", ExerciseCategory.TOTAL_BODY, 0, meditation_duration, 1.2, True)
    for _ in range(num_meditations):
        schedule.append(meditation_entry)

    total_sets = len(schedule)

    # Pre-calculate total time
    total_active_time = sum(pose[3] for pose in schedule)
    total_rest_time = (total_sets - 1) * rest_seconds
    total_time_s = total_active_time + total_rest_time

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

    # --- 3. Exercise Titles ---
    seen_exercises = {}
    for pose in schedule:
        key = (pose[0], pose[1].value)  # (name, category_value) as unique key
        if key not in seen_exercises:
            idx = len(seen_exercises)
            seen_exercises[key] = idx
            title = ExerciseTitleMessage()
            title.message_index = idx
            title.exercise_category = pose[1].value
            title.exercise_name = pose[2]
            title.workout_step_name = pose[0]
            builder.add(title)

    # --- 4. Timer Start ---
    start_event = EventMessage()
    start_event.event = Event.TIMER
    start_event.event_type = EventType.START
    start_event.timestamp = start_timestamp_ms
    builder.add(start_event)

    # --- 5. Generate Records + Set messages ---
    #
    # Yoga HR model:
    #   - Resting HR ~62-70 bpm (yoga practitioners tend lower)
    #   - Active poses: HR rises gently to 85-110 bpm
    #   - Meditation: HR drops to near-resting, 60-72 bpm
    #   - Transitions: slight dip
    #   - Hard cap at 130 bpm for yoga
    #
    RESTING_HR = random.uniform(62, 70)
    MAX_HR_CAP = 130
    hr = RESTING_HR
    records = []
    total_calories = 0
    all_hr_values = []

    for set_num, pose in enumerate(schedule):
        pose_name, pose_cat, pose_name_id, hold_time, kcal_per_min, is_meditation = pose
        set_start_ts = current_ts

        if is_meditation:
            # Meditation: HR drifts down to near-resting
            target_hr = RESTING_HR + random.uniform(-2, 3)
        else:
            # Active pose: target depends on intensity
            target_hr = min(85 + kcal_per_min * 8, MAX_HR_CAP - 5)
            # Progressive slight increase across the session
            target_hr += min(set_num * 0.5, 8)

        # --- Hold: generate 1 record per second ---
        for sec in range(hold_time):
            record = RecordMessage()
            record.timestamp = current_ts

            # Gradual convergence toward target HR
            converge_rate = 0.02 if is_meditation else random.uniform(0.04, 0.10)
            hr += (target_hr - hr) * converge_rate

            # Smaller jitter for meditation, slightly more for active
            jitter = random.uniform(-0.5, 0.5) if is_meditation else random.uniform(-1.0, 1.0)
            hr += jitter
            hr = max(RESTING_HR - 5, min(MAX_HR_CAP, hr))

            record.heart_rate = round(hr)
            all_hr_values.append(round(hr))
            records.append(record)
            current_ts += 1000

        # --- Set message ---
        set_msg = SetMessage()
        set_msg.timestamp = current_ts
        set_msg.start_time = set_start_ts
        set_msg.duration = hold_time
        set_msg.set_type = SetType.ACTIVE
        set_msg.category = [pose_cat.value]
        set_msg.category_subtype = [pose_name_id]
        set_msg.repetitions = 1  # 1 hold = 1 rep for yoga
        set_msg.message_index = set_num
        records.append(set_msg)

        total_calories += round(hold_time / 60.0 * kcal_per_min)

        # --- Transition rest (except after last pose) ---
        if set_num < total_sets - 1:
            # During transition, HR drifts slightly down
            transition_target = hr - random.uniform(2, 5)
            for sec in range(rest_seconds):
                record = RecordMessage()
                record.timestamp = current_ts

                hr += (transition_target - hr) * 0.05
                hr += random.uniform(-0.5, 0.5)
                hr = max(RESTING_HR - 5, min(MAX_HR_CAP, hr))

                record.heart_rate = round(hr)
                all_hr_values.append(round(hr))
                records.append(record)
                current_ts += 1000

    builder.add_all(records)

    # Compute HR stats
    avg_hr = round(sum(all_hr_values) / len(all_hr_values)) if all_hr_values else 0
    max_hr = max(all_hr_values) if all_hr_values else 0
    min_hr = min(all_hr_values) if all_hr_values else 0

    # --- 6. Timer Stop ---
    stop_event = EventMessage()
    stop_event.event = Event.TIMER
    stop_event.event_type = EventType.STOP
    stop_event.timestamp = current_ts
    builder.add(stop_event)

    # --- 7. Lap ---
    lap = LapMessage()
    lap.timestamp = current_ts
    lap.start_time = start_timestamp_ms
    lap.total_elapsed_time = total_time_s
    lap.total_timer_time = total_time_s
    lap.total_calories = total_calories
    lap.avg_heart_rate = avg_hr
    lap.max_heart_rate = max_hr
    lap.sport = Sport.TRAINING
    lap.sub_sport = SubSport.YOGA
    lap.lap_trigger = LapTrigger.SESSION_END
    builder.add(lap)

    # --- 8. Session ---
    session = SessionMessage()
    session.timestamp = current_ts
    session.start_time = start_timestamp_ms
    session.total_elapsed_time = total_time_s
    session.total_timer_time = total_time_s
    session.sport = Sport.TRAINING
    session.sub_sport = SubSport.YOGA
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
    activity.total_timer_time = total_time_s
    activity.num_sessions = 1
    activity.type = Activity.MANUAL
    activity.event = Event.ACTIVITY
    activity.event_type = EventType.STOP
    builder.add(activity)

    # Build and write
    fit_file = builder.build()
    fit_file.to_file(args.file)

    hours = total_time_s // 3600
    minutes = (total_time_s % 3600) // 60
    seconds = total_time_s % 60
    print(f"Yoga session summary:")
    print(f"  Poses:      {num_poses} active + {num_meditations} meditation")
    print(f"  Transition: {rest_seconds}s between poses")
    print(f"  Duration:   {hours:02d}:{minutes:02d}:{seconds:02d}")
    print(f"  Calories:   ~{total_calories} kcal")
    print(f"  Heart Rate: avg {avg_hr} / max {max_hr} / min {min_hr} bpm")
    print(f"  Sequence:")
    for i, pose in enumerate(schedule):
        tag = " [Meditation]" if pose[5] else ""
        print(f"    {i+1}. {pose[0]} ({pose[3]}s){tag}")
    print(f"Successfully generated {args.file}")


if __name__ == "__main__":
    main()
