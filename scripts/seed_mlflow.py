#!/usr/bin/env python3
"""Seed a local MLflow tracking server with runs that exercise this MCP server.

The runs here are shaped to hit the edges the Go client and tools care about:
latest-value-per-metric, empty runs, non-active statuses, and one deliberately
large payload for measuring output size.

Usage:
    source .venv/bin/activate
    mlflow server --backend-store-uri sqlite:///mlflow.db --port 5000 &
    python scripts/seed_mlflow.py                  # add fixtures
    python scripts/seed_mlflow.py --reset          # drop fixtures first

Environment:
    MLFLOW_TRACKING_URI   defaults to http://localhost:5000
"""

import argparse
import os
import sys

# Keep mlflow's import-time agent hint and per-run "View run at" links out of
# the seeding output.
os.environ.setdefault("MLFLOW_DISABLE_AGENT_HINT", "1")
os.environ.setdefault("MLFLOW_SUPPRESS_PRINTING_URL_TO_STDOUT", "1")

import mlflow
from mlflow import MlflowClient

MAIN_EXPERIMENT = "mlflow-mcp-fixtures"
ABLATION_EXPERIMENT = "mlflow-mcp-fixtures-ablation"


def log_curve(metric, points):
    """Log every point of a metric.

    MLflow stores all of them, but `runs/get` reports only the value with the
    latest timestamp per key. That asymmetry is exactly what the summarised
    `get_run` output has to hide from the model.
    """
    for step, value in points:
        mlflow.log_metric(metric, value, step=step)


def seed_main_experiment():
    exp = mlflow.set_experiment(MAIN_EXPERIMENT)
    print(f"experiment {exp.name!r} id={exp.experiment_id}")
    run_ids = []

    # 1. Baseline. Moderate params, monotonically improving metrics.
    with mlflow.start_run(run_name="baseline-lr-0.01") as run:
        mlflow.log_params(
            {"model": "logreg", "lr": "0.01", "epochs": "5", "batch_size": "64"}
        )
        log_curve("auc", [(0, 0.61), (1, 0.72), (2, 0.80), (3, 0.85), (4, 0.88)])
        log_curve("loss", [(0, 0.98), (1, 0.71), (2, 0.55), (3, 0.48), (4, 0.44)])
        mlflow.set_tag("team", "ml-platform")
        mlflow.set_tag("env", "dev")
        run_ids.append(run.info.run_id)

    # 2. Same model, higher lr. Best auc: the obvious `compare_runs` winner,
    #    and the target of a "best by metrics.auc" filter.
    with mlflow.start_run(run_name="tuned-lr-0.1") as run:
        mlflow.log_params(
            {"model": "logreg", "lr": "0.1", "epochs": "5", "batch_size": "64"}
        )
        log_curve("auc", [(0, 0.70), (1, 0.84), (2, 0.90), (3, 0.92), (4, 0.93)])
        log_curve("loss", [(0, 0.85), (1, 0.52), (2, 0.38), (3, 0.33), (4, 0.30)])
        mlflow.set_tag("team", "ml-platform")
        mlflow.set_tag("env", "dev")
        run_ids.append(run.info.run_id)

    # 3. Different algorithm with an extra param, so param diffs are not just
    #    "value differs" but "key missing on one side".
    with mlflow.start_run(run_name="rf-500-trees") as run:
        mlflow.log_params(
            {"model": "random_forest", "n_estimators": "500", "max_depth": "12"}
        )
        log_curve("auc", [(0, 0.91), (1, 0.94), (2, 0.955)])
        log_curve("loss", [(0, 0.31), (1, 0.22), (2, 0.19)])
        mlflow.set_tag("team", "research")
        run_ids.append(run.info.run_id)

    # 4. Failed run: proves status is a real field, not a constant. Metrics
    #    logged before the crash survive.
    run = mlflow.start_run(run_name="crashed-on-oom")
    mlflow.log_params({"model": "xgboost", "n_estimators": "1000"})
    log_curve("loss", [(0, 1.20), (1, 0.94)])
    run_ids.append(run.info.run_id)
    mlflow.end_run(status="FAILED")

    # 5. Bare run: no params, no metrics, no tags, no name. Tests that the
    #    summary degrades instead of printing empty scaffolding. Note the
    #    server may auto-assign a run_name here.
    with mlflow.start_run() as run:
        run_ids.append(run.info.run_id)

    # 6. Deliberately large: 500 metric points across 5 keys plus a 2 KB param.
    #    Milestone 4 compares the summarised `get_run` against this raw payload.
    with mlflow.start_run(run_name="long-sweep") as run:
        mlflow.log_params(
            {
                "model": "logreg",
                "lr": "0.02",
                "epochs": "100",
                "note": "x" * 2000,
            }
        )
        for key, base in (("auc", 0.55), ("loss", 1.05), ("f1", 0.40)):
            log_curve(
                key,
                [(step, round(base + step * 0.0004, 6)) for step in range(500)],
            )
        mlflow.set_tag("team", "ml-platform")
        run_ids.append(run.info.run_id)

    return exp.experiment_id, run_ids


def seed_ablation_experiment():
    """A second experiment, so `list_experiments` has more than one row."""
    exp = mlflow.set_experiment(ABLATION_EXPERIMENT)
    print(f"experiment {exp.name!r} id={exp.experiment_id}")
    run_ids = []

    for name, dropout in (("with-dropout", "0.3"), ("no-dropout", "0.0")):
        with mlflow.start_run(run_name=name) as run:
            mlflow.log_params({"model": "mlp", "dropout": dropout, "epochs": "20"})
            log_curve("auc", [(0, 0.66), (1, 0.75), (2, 0.79)])
            run_ids.append(run.info.run_id)

    return exp.experiment_id, run_ids


def reset(experiment_names):
    """Mark fixture runs deleted so a re-seed starts clean.

    This sets lifecycle_stage=deleted; hard deletion needs `mlflow gc`.
    """
    client = MlflowClient()
    for name in experiment_names:
        exp = client.get_experiment_by_name(name)
        if exp is None:
            print(f"reset: {name!r} does not exist, nothing to do")
            continue
        runs = client.search_runs([exp.experiment_id], max_results=10000)
        for run in runs:
            client.delete_run(run.info.run_id)
        print(f"reset: deleted {len(runs)} run(s) from {name!r}")


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument(
        "--tracking-uri",
        default=os.environ.get("MLFLOW_TRACKING_URI", "http://localhost:5000"),
        help="tracking server to seed (default: %(default)s)",
    )
    parser.add_argument(
        "--reset",
        action="store_true",
        help="delete existing fixture runs before seeding",
    )
    args = parser.parse_args()

    mlflow.set_tracking_uri(args.tracking_uri)
    print(f"tracking uri: {args.tracking_uri}")

    names = [MAIN_EXPERIMENT, ABLATION_EXPERIMENT]
    try:
        if args.reset:
            reset(names)
        main_id, main_runs = seed_main_experiment()
        abl_id, abl_runs = seed_ablation_experiment()
    except Exception as exc:  # noqa: BLE001 - one readable line beats a traceback
        print(
            f"\nseeding failed: {exc}\n"
            "Is the server up? See the dev loop in AGENTS.md:\n"
            "  mlflow server --backend-store-uri sqlite:///mlflow.db --port 5000",
            file=sys.stderr,
        )
        return 1

    print(f"\nseeded {len(main_runs) + len(abl_runs)} runs")
    print(f"  {MAIN_EXPERIMENT}    id={main_id}")
    for run_id in main_runs:
        print(f"    {run_id}")
    print(f"  {ABLATION_EXPERIMENT}    id={abl_id}")
    for run_id in abl_runs:
        print(f"    {run_id}")
    print("\nPaste one into main.go, e.g. GetRun(ctx, \"" + main_runs[0] + "\")")
    return 0


if __name__ == "__main__":
    sys.exit(main())
