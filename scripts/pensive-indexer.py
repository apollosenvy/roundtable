#!/usr/bin/env python3
"""
Pensive Debate Indexer

Indexes pending debate records from the roundtable fallback directory into Pensive L2.
Run this periodically or when Pensive becomes available after being offline.

Usage:
    python pensive-indexer.py                    # Index all pending debates
    python pensive-indexer.py --dry-run          # Preview what would be indexed
    python pensive-indexer.py --service          # Run as a service, watching for new files
"""

import json
import os
import sys
import time
import argparse
import logging
from pathlib import Path
from typing import List, Dict, Any, Optional

# Add Pensive to path if available
try:
    pensive_path = Path.home() / "Projects" / "Pensive"
    if pensive_path.exists():
        sys.path.insert(0, str(pensive_path))
        from kv_cache.retriever import embedder, VECTOR_URL
        import requests
        PENSIVE_AVAILABLE = True
    else:
        PENSIVE_AVAILABLE = False
except ImportError:
    PENSIVE_AVAILABLE = False

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# Default paths
DEFAULT_PENDING_DIR = Path.home() / ".local" / "share" / "roundtable" / "pensive-pending"
DEFAULT_INDEXED_DIR = Path.home() / ".local" / "share" / "roundtable" / "pensive-indexed"
VECTOR_SERVICE_URL = os.getenv("VECTOR_URL", "http://127.0.0.1:8009")


def load_pending_debates(pending_dir: Path) -> List[Dict[str, Any]]:
    """Load all pending debate records from the fallback directory."""
    debates = []

    if not pending_dir.exists():
        logger.info(f"No pending directory at {pending_dir}")
        return debates

    for file_path in pending_dir.glob("debate_*.json"):
        try:
            with open(file_path, 'r') as f:
                record = json.load(f)
                record['_source_file'] = str(file_path)
                debates.append(record)
        except Exception as e:
            logger.error(f"Failed to load {file_path}: {e}")

    logger.info(f"Found {len(debates)} pending debates")
    return debates


def check_pensive_available() -> bool:
    """Check if the Pensive vector service is available."""
    try:
        response = requests.get(f"{VECTOR_SERVICE_URL}/stats", timeout=5)
        return response.status_code == 200
    except:
        return False


def index_debate(record: Dict[str, Any]) -> bool:
    """Index a single debate record into Pensive."""
    if not PENSIVE_AVAILABLE:
        logger.error("Pensive not available - cannot create embeddings")
        return False

    try:
        # Create embedding from the summary
        summary = record.get('summary', '')
        if not summary:
            # Fall back to first 500 chars of transcript
            summary = record.get('transcript', '')[:500]

        embedding = embedder.encode(summary, normalize_embeddings=True).tolist()

        # Prepare metadata
        meta = {
            "hash": record.get('hash', ''),
            "debate_id": record.get('debate_id', ''),
            "debate_name": record.get('debate_name', ''),
            "summary": summary[:1000],  # Truncate for storage
            "consensus": record.get('consensus', ''),
            "dates": record.get('dates', []),
            "entities": record.get('entities', []),
            "outcome": record.get('outcome', 'unknown'),
            "participants": ",".join(record.get('participants', [])),
        }

        # Send to vector service
        response = requests.post(
            f"{VECTOR_SERVICE_URL}/insert",
            json={
                "vectors": [embedding],
                "metas": [meta]
            },
            timeout=30
        )

        if response.status_code >= 400:
            logger.error(f"Pensive returned {response.status_code}: {response.text}")
            return False

        logger.info(f"Indexed debate {record.get('debate_id')}: {record.get('debate_name')}")
        return True

    except Exception as e:
        logger.error(f"Failed to index debate {record.get('debate_id')}: {e}")
        return False


def move_to_indexed(source_file: Path, indexed_dir: Path):
    """Move a successfully indexed file to the indexed directory."""
    indexed_dir.mkdir(parents=True, exist_ok=True)
    dest = indexed_dir / source_file.name
    source_file.rename(dest)
    logger.debug(f"Moved {source_file.name} to indexed directory")


def index_all_pending(pending_dir: Path, indexed_dir: Path, dry_run: bool = False) -> tuple:
    """Index all pending debates and move them to indexed directory."""
    debates = load_pending_debates(pending_dir)

    if not debates:
        return 0, 0

    if dry_run:
        logger.info(f"DRY RUN: Would index {len(debates)} debates:")
        for d in debates:
            logger.info(f"  - {d.get('debate_id')}: {d.get('debate_name')}")
        return len(debates), 0

    if not check_pensive_available():
        logger.error("Pensive vector service not available")
        return 0, len(debates)

    indexed = 0
    failed = 0

    for record in debates:
        source_file = Path(record['_source_file'])
        del record['_source_file']  # Don't include in indexed data

        if index_debate(record):
            move_to_indexed(source_file, indexed_dir)
            indexed += 1
        else:
            failed += 1

    return indexed, failed


def run_service(pending_dir: Path, indexed_dir: Path, interval: int = 60):
    """Run as a service, periodically checking for new debates to index."""
    logger.info(f"Starting indexer service (interval: {interval}s)")
    logger.info(f"Watching: {pending_dir}")

    while True:
        try:
            if check_pensive_available():
                indexed, failed = index_all_pending(pending_dir, indexed_dir)
                if indexed > 0 or failed > 0:
                    logger.info(f"Indexed: {indexed}, Failed: {failed}")
            else:
                logger.debug("Pensive not available, waiting...")
        except KeyboardInterrupt:
            logger.info("Service stopped")
            break
        except Exception as e:
            logger.error(f"Service error: {e}")

        time.sleep(interval)


def main():
    parser = argparse.ArgumentParser(description="Index roundtable debates into Pensive")
    parser.add_argument('--dry-run', action='store_true', help="Preview what would be indexed")
    parser.add_argument('--service', action='store_true', help="Run as a service")
    parser.add_argument('--interval', type=int, default=60, help="Service polling interval (seconds)")
    parser.add_argument('--pending-dir', type=str, default=str(DEFAULT_PENDING_DIR),
                        help="Directory containing pending debates")
    parser.add_argument('--indexed-dir', type=str, default=str(DEFAULT_INDEXED_DIR),
                        help="Directory to move indexed debates")
    parser.add_argument('--verbose', '-v', action='store_true', help="Verbose output")

    args = parser.parse_args()

    if args.verbose:
        logging.getLogger().setLevel(logging.DEBUG)

    pending_dir = Path(args.pending_dir)
    indexed_dir = Path(args.indexed_dir)

    if not PENSIVE_AVAILABLE and not args.dry_run:
        logger.error("Pensive not available. Make sure the Pensive project is at ~/Projects/Pensive")
        logger.error("and sentence-transformers is installed.")
        sys.exit(1)

    if args.service:
        run_service(pending_dir, indexed_dir, args.interval)
    else:
        indexed, failed = index_all_pending(pending_dir, indexed_dir, args.dry_run)
        if args.dry_run:
            print(f"Would index {indexed} debates")
        else:
            print(f"Indexed: {indexed}, Failed: {failed}")
            sys.exit(1 if failed > 0 else 0)


if __name__ == "__main__":
    main()
