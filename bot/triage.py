"""
AI severity-triage layer for the safety-incident bot.

Runs ALWAYS-ON (not human-flagged) against every incoming incident report,
classifying severity per the agreed 4-tier rubric and weighing injury risk,
equipment damage, and near-miss/systemic signals. Medium and above get
logged on-chain by the caller (see chain.py).
"""

import os
from dataclasses import dataclass
from enum import IntEnum
from typing import Literal

from dotenv import load_dotenv
from google import genai
from google.genai import types
from pydantic import BaseModel, Field

# Supported production flash model
MODEL = "gemini-3.6-flash"


class Severity(IntEnum):
    LOW = 0
    MEDIUM = 1
    HIGH = 2
    CRITICAL = 3


# Only Medium+ gets written on-chain, per the agreed threshold.
ON_CHAIN_THRESHOLD = Severity.MEDIUM


# Define Pydantic schema using Literal to enforce exact string enums from Gemini
class TriageResponseSchema(BaseModel):
    severity: Literal["LOW", "MEDIUM", "HIGH", "CRITICAL"] = Field(
        description="Must be LOW, MEDIUM, HIGH, or CRITICAL"
    )
    driving_factor: Literal["injury", "equipment", "near_miss"] = Field(
        description="Must be injury, equipment, or near_miss"
    )
    summary: str = Field(description="One sentence, plain language, what happened and where")
    justification: str = Field(description="One sentence explaining why this tier was chosen")


RUBRIC_PROMPT = """You are a workplace safety triage assistant. Classify the
severity of the incident report below into exactly one of four tiers, using
this rubric:

LOW — No injury or risk described. Cosmetic/minor equipment issue
(scratch, minor spill). Routine observation, no near-miss signal.

MEDIUM — Minor injury possible (bruise, cut) or discomfort reported.
Functional equipment damage, repairable, no shutdown needed. Hazard was
present but avoided through normal caution.

HIGH — Injury occurred, or serious injury is plausible (fracture, burn,
chemical exposure). Major equipment damage requiring shutdown/repair.
Hazard was avoided narrowly / by luck rather than by design (a genuine
near-miss).

CRITICAL — Life-threatening, fatality risk, or multiple people at risk.
Catastrophic failure risk (structural, fire, explosion). The report
reveals a systemic gap where the same hazard is likely to recur
immediately.

Weigh THREE factors independently: injury risk, equipment damage, and
near-miss/systemic signal. The severity is driven by whichever factor is
most severe — a report with no injury can still be HIGH or CRITICAL if it
reveals a serious systemic near-miss.

Incident report:
\"\"\"{report_text}\"\"\"
"""


@dataclass
class TriageResult:
    severity: Severity
    driving_factor: str
    summary: str
    justification: str
    raw_report: str

    def should_log_on_chain(self) -> bool:
        return self.severity >= ON_CHAIN_THRESHOLD

    def record_payload(self) -> dict:
        """Canonical off-chain record. Its hash is what goes on-chain."""
        return {
            "summary": self.summary,
            "severity": self.severity.name,
            "driving_factor": self.driving_factor,
            "justification": self.justification,
        }


def _get_client() -> genai.Client:
    """Instantiate client, explicitly ensuring environment variables are loaded."""
    # Force load of .env file from the current working directory
    load_dotenv()
    
    # Check for the new variable name, then fall back to the old one
    api_key = os.environ.get("GEMINI_API_KEY") or os.environ.get("GOOGLE_API_KEY")
    
    if not api_key:
        raise ValueError(
            "API key not found! Please ensure you have a .env file containing "
            "GEMINI_API_KEY=your_key_here, or that it is exported in your terminal."
        )
    
    # Clean up any accidental spaces, newlines, or quotes from the .env file
    api_key = api_key.strip(' "\'\n')
        
    return genai.Client(api_key=api_key)


def classify_incident(report_text: str) -> TriageResult:
    """Always-on AI severity classification for a single incident report."""
    client = _get_client()
    response = client.models.generate_content(
        model=MODEL,
        contents=RUBRIC_PROMPT.format(report_text=report_text),
        config=types.GenerateContentConfig(
            response_mime_type="application/json",
            response_schema=TriageResponseSchema,
            temperature=0.1,  # Low temperature for deterministic classification
        ),
    )

    data: TriageResponseSchema = response.parsed

    return TriageResult(
        severity=Severity[data.severity.upper()],
        driving_factor=data.driving_factor,
        summary=data.summary,
        justification=data.justification,
        raw_report=report_text,
    )


if __name__ == "__main__":
    # Quick manual smoke test against a few sample reports.
    samples = [
        "Spilled some coffee near my desk, cleaned it up right away.",
        "Forklift in bay 3 nearly clipped me, I jumped back just in time. Guard rail there has been missing for weeks.",
        "Got a small cut on my hand from a box cutter, put a bandage on it.",
        "Chemical tank in storage room B is hissing and there's a smell, two of us are near it right now.",
    ]
    for text in samples:
        result = classify_incident(text)
        print(f"\n[{result.severity.name}] ({result.driving_factor}) -> log_on_chain={result.should_log_on_chain()}")
        print(f"  summary: {result.summary}")
        print(f"  why: {result.justification}")