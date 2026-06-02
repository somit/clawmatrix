import os
import sys
from functools import cached_property

from google.adk.agents.llm_agent import Agent
from google.adk.models.google_llm import Gemini
from google.genai import Client

sys.path.insert(0, "/opt/adk-agent")
from clawmatrix_tools import delegate_to_clawmatrix_agent, list_clawmatrix_agents

AGENT_NAME = "Sales Manager"
AGENT_ROLE = (
    "Sales strategy, pipeline management, outreach, qualification, competitive "
    "positioning, and revenue forecasting."
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
    name="sales_manager",
    model=model(),
    instruction=(
        "You are Sales Manager in the ClawMatrix team. Provide concise, "
        "practical sales strategy, pipeline management, outreach, qualification, "
        "competitive positioning, and revenue forecasting. You can use ClawMatrix "
        "tools through clutch. When the user asks what agents you can reach, call "
        "list_clawmatrix_agents. When the user asks you to contact, connect to, "
        "ask, or delegate to another agent, call delegate_to_clawmatrix_agent "
        "with the requested target and a concise task message. For sales leads, "
        "pipeline details, qualification, revenue, sales priorities, or outreach "
        "requests, answer directly as Sales Manager; do not delegate those "
        "sales-owned requests to marketing-manager. If no live CRM data is "
        "available, say that and provide a concise demo pipeline summary or "
        "recommended lead-prioritization plan. Only delegate to marketing-manager "
        "for marketing-owned questions such as campaign strategy, positioning, "
        "brand voice, SEO, or content. If the tool says the target is not "
        "authorized, explain that the current ClawMatrix connection graph does "
        "not allow that handoff."
    ),
    tools=[list_clawmatrix_agents, delegate_to_clawmatrix_agent],
)
