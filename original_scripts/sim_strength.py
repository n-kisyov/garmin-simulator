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

# Exercise pool: (display_name, ExerciseCategory enum, exercise_name_id, secs_per_rep, kcal_per_rep)
EXERCISES = [
    ("Push Up",   ExerciseCategory.PUSH_UP, 77, 2.5, 0.50),   # PushUpExerciseName.PUSH_UP
    ("Squat",     ExerciseCategory.SQUAT,   61, 3.0, 0.55),   # SquatExerciseName.SQUAT
    ("Lunge",     ExerciseCategory.LUNGE,   32, 3.0, 0.50),   # LungeExerciseName.LUNGE
    ("Crunch",    ExerciseCategory.CRUNCH,  83, 2.0, 0.25),   # CrunchExerciseName.CRUNCH
    ("Pull Up",   ExerciseCategory.PULL_UP, 38, 3.5, 0.65),   # PullUpExerciseName.PULL_UP
    ("Sit Up",    ExerciseCategory.SIT_UP,  37, 2.5, 0.30),   # SitUpExerciseName.SIT_UP
]


def main():
    parser = argparse.ArgumentParser(
        description="Simulate a strength training FIT file for Garmin devices. "
                    "Rotates through exercises: Push Up, Squat, Lunge, Crunch, Pull Up, Sit Up.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""Examples:
  6 sets rotating through all exercises (12 reps each):
    python sim_strength.py --datetime "01-04-26 18:00:00" --reps 12 --sets 6 --file strength.fit

  12 sets = 2 full rotations, 15 reps, 90s rest:
    python sim_strength.py --datetime "01-04-26 18:00:00" --reps 15 --sets 12 --rest 90 --file strength.fit
"""
    )
    parser.add_argument("--datetime", type=str, required=True,
                        help="Start datetime in format 'dd-mm-yy HH:MM:SS'")
    parser.add_argument("--file", type=str, required=True,
                        help="Output FIT filename")
    parser.add_argument("--reps", type=int, required=True,
                        help="Number of reps per set")
    parser.add_argument("--sets", type=int, default=6,
                        help="Total number of sets (default: 6 = one full rotation)")
    parser.add_argument("--rest", type=int, default=60,
                        help="Rest time between sets in seconds (default: 60)")

    args = parser.parse_args()

    try:
        start_time = datetime.datetime.strptime(args.datetime, "%d-%m-%y %H:%M:%S")
    except ValueError:
        print(f"Error parsing datetime. Expected 'dd-mm-yy HH:MM:SS'. Got '{args.datetime}'")
        return

    num_sets = args.sets
    reps_per_set = args.reps
    rest_seconds = args.rest

    # Build the schedule: rotate through exercises
    schedule = []
    for s in range(num_sets):
        exercise = EXERCISES[s % len(EXERCISES)]
        schedule.append(exercise)

    # Pre-calculate total workout time
    total_active_time = sum(int(reps_per_set * ex[3]) for ex in schedule)
    total_rest_time = (num_sets - 1) * rest_seconds
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

    # --- 3. Exercise Titles (one per unique exercise in the schedule) ---
    seen_exercises = {}
    for ex in schedule:
        cat_value = ex[1].value
        if cat_value not in seen_exercises:
            idx = len(seen_exercises)
            seen_exercises[cat_value] = idx
            title = ExerciseTitleMessage()
            title.message_index = idx
            title.exercise_category = cat_value
            title.exercise_name = ex[2]
            title.workout_step_name = ex[0]
            builder.add(title)

    # --- 4. Timer Start ---
    start_event = EventMessage()
    start_event.event = Event.TIMER
    start_event.event_type = EventType.START
    start_event.timestamp = start_timestamp_ms
    builder.add(start_event)

    # --- 5. Generate Records + Set messages ---
    #
    # Realistic HR model for strength training:
    #   - Resting HR ~68-75 bpm at workout start
    #   - During a set: HR rises toward a target that increases with each set
    #     (cumulative fatigue effect).
    #   - During rest: HR drops exponentially toward a "recovery floor" that
    #     also rises slightly across the workout (incomplete recovery).
    #   - Small beat-to-beat jitter (±1-2 bpm) for realism.
    #   - Hard cap at 165 bpm.
    #
    RESTING_HR = random.uniform(68, 75)
    MAX_HR_CAP = 165
    hr = RESTING_HR
    records = []
    total_reps = 0
    total_calories = 0
    all_hr_values = []

    for set_num, exercise in enumerate(schedule):
        ex_name, ex_cat, ex_name_id, secs_per_rep, kcal_per_rep = exercise
        set_duration_s = int(reps_per_set * secs_per_rep)
        set_start_ts = current_ts

        # Target peak HR for this set — rises with each successive set
        fatigue_factor = (set_num + 1) / num_sets
        set_peak_hr = 115 + fatigue_factor * 45   # range ~130 to 160
        set_peak_hr = min(set_peak_hr, MAX_HR_CAP)

        # --- Active set: generate 1 record per second ---
        for sec in range(set_duration_s):
            record = RecordMessage()
            record.timestamp = current_ts

            # HR rises toward set_peak_hr with diminishing acceleration
            progress = (sec + 1) / set_duration_s
            target = hr + (set_peak_hr - hr) * (1 - (1 - progress) ** 1.5)
            climb_rate = random.uniform(0.15, 0.30)
            hr += (target - hr) * climb_rate
            hr += random.uniform(-1.2, 1.2)  # beat-to-beat jitter
            hr = max(RESTING_HR, min(MAX_HR_CAP, hr))

            record.heart_rate = round(hr)
            all_hr_values.append(round(hr))
            records.append(record)
            current_ts += 1000

        # --- Set message ---
        set_msg = SetMessage()
        set_msg.timestamp = current_ts
        set_msg.start_time = set_start_ts
        set_msg.duration = set_duration_s
        set_msg.set_type = SetType.ACTIVE
        set_msg.category = [ex_cat.value]
        set_msg.category_subtype = [ex_name_id]
        set_msg.repetitions = reps_per_set
        set_msg.message_index = set_num
        records.append(set_msg)

        total_reps += reps_per_set
        total_calories += round(reps_per_set * kcal_per_rep)

        # --- Rest period (except after last set) ---
        if set_num < num_sets - 1:
            recovery_floor = RESTING_HR + 10 + set_num * 3
            recovery_floor = min(recovery_floor, 100)

            for sec in range(rest_seconds):
                record = RecordMessage()
                record.timestamp = current_ts

                decay_rate = random.uniform(0.03, 0.06)
                hr += (recovery_floor - hr) * decay_rate
                hr += random.uniform(-0.8, 0.8)
                hr = max(RESTING_HR - 3, min(MAX_HR_CAP, hr))

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
    lap.sub_sport = SubSport.STRENGTH_TRAINING
    lap.lap_trigger = LapTrigger.SESSION_END
    builder.add(lap)

    # --- 8. Session ---
    session = SessionMessage()
    session.timestamp = current_ts
    session.start_time = start_timestamp_ms
    session.total_elapsed_time = total_time_s
    session.total_timer_time = total_time_s
    session.sport = Sport.TRAINING
    session.sub_sport = SubSport.STRENGTH_TRAINING
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
    print(f"Workout summary:")
    print(f"  Sets:       {num_sets}")
    print(f"  Reps/set:   {reps_per_set}")
    print(f"  Total reps: {total_reps}")
    print(f"  Rest:       {rest_seconds}s between sets")
    print(f"  Duration:   {hours:02d}:{minutes:02d}:{seconds:02d}")
    print(f"  Calories:   ~{total_calories} kcal")
    print(f"  Heart Rate: avg {avg_hr} / max {max_hr} / min {min_hr} bpm")
    print(f"  Exercises:")
    for i, ex in enumerate(schedule):
        print(f"    Set {i+1}: {ex[0]} x{reps_per_set}")
    print(f"Successfully generated {args.file}")


if __name__ == "__main__":
    main()
