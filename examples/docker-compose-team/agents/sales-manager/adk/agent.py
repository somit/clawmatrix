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
        "competitive positioning, and revenue forecasting. Answer sales-owned "
        "requests yourself. If no live CRM data is available, say so and provide "
        "a concise demo pipeline summary or recommended lead-prioritization plan. "
        "You can reach other ClawMatrix agents through clutch, but you do not "
        "know in advance who they are or what they do. When a request falls "
        "outside sales — or the user asks you to contact, connect to, ask, or "
        "delegate to another agent — first call list_clawmatrix_agents to see "
        "which agents you are authorized to reach and read each one's "
        "description. Choose the single agent whose description best matches the "
        "request, then call delegate_to_clawmatrix_agent with that agent's name "
        "and a concise task message. Do not assume a particular agent exists; "
        "rely only on what list_clawmatrix_agents returns. If no listed agent "
        "fits, or the list is empty, tell the user you have no authorized agent "
        "for that request instead of guessing."
    ),
    tools=[list_clawmatrix_agents, delegate_to_clawmatrix_agent],
)
