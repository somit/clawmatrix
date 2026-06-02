import os
import sys
from functools import cached_property

from google.adk.agents.llm_agent import Agent
from google.adk.models.google_llm import Gemini
from google.genai import Client

sys.path.insert(0, "/opt/adk-agent")
from clawmatrix_tools import delegate_to_clawmatrix_agent, list_clawmatrix_agents

AGENT_NAME = "Marketing Manager"
AGENT_ROLE = (
    "Marketing strategy, campaign planning, positioning, brand voice, SEO, "
    "and content guidance."
)

DEFAULT_MODEL = "@google/global/gemini-3-flash-preview"


def model():
    model_id = os.getenv("ADK_MODEL", DEFAULT_MODEL)
    region, bare_model = parse_google_model(model_id)
    if not region:
        return Gemini(model=bare_model)

    class RegionGemini(Gemini):
        @cached_property
        def api_client(self):
            return Client(
                vertexai=True,
                project=os.getenv("GOOGLE_CLOUD_PROJECT"),
                location=region,
            )

    return RegionGemini(model=bare_model)


def parse_google_model(model_id: str) -> tuple[str, str]:
    if model_id.startswith("@google/"):
        parts = model_id.split("/", 2)
        if len(parts) == 3:
            return parts[1], parts[2]
    return "", model_id


root_agent = Agent(
    name="marketing_manager",
    model=model(),
    instruction=(
        "You are Marketing Manager in the ClawMatrix team. Provide concise, "
        "practical marketing strategy, campaign planning, positioning, brand "
        "voice, SEO, and content guidance. You can use ClawMatrix tools through "
        "clutch. When the user asks what agents you can reach, call "
        "list_clawmatrix_agents. When the user asks you to contact, connect to, "
        "ask, or delegate to another agent, call delegate_to_clawmatrix_agent "
        "with the requested target and a concise task message. When the user "
        "asks for sales leads, pipeline details, qualification, revenue, sales "
        "priorities, or outreach help, call delegate_to_clawmatrix_agent with "
        "the exact target string sales-manager even if the user did not "
        "explicitly say delegate. "
        "If the tool says the target is not authorized, explain that the "
        "current ClawMatrix connection graph does not allow that handoff."
    ),
    tools=[list_clawmatrix_agents, delegate_to_clawmatrix_agent],
)
