import ray
import argparse
import time


@ray.remote
def test_task(x, delay):
    time.sleep(delay)  # Simulate work
    return f"Task {x} completed after {delay} seconds!"


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Submit a test job to Ray with multiple arguments.")
    parser.add_argument("--tasks", type=int, default=5, help="Number of tasks to run in parallel.")
    parser.add_argument("--delay", type=float, default=1.0, help="Delay time per task in seconds.")
    args = parser.parse_args()
    print("\n".join(ray.get([test_task.remote(i, args.delay) for i in range(args.tasks)])))
