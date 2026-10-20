const apiBase = import.meta.env.VITE_API_URL ?? "http://localhost:8080";

export type AgentEvent = { dateId?: string; sequence?: number; type: string; payload: Record<string, unknown> };
export type ObservatoryAgent = { id: string; name: string; tags: string[]; skills: string[] };
export type PublicDate = { id: string; scenario: string; status: string; currentTurn: number; maxTurns: number; simulatedTime: number; participants: ObservatoryAgent[]; events: AgentEvent[]; summary?: string; createdAt: string };

export async function publicDates(): Promise<PublicDate[]> {
	const response = await fetch(`${apiBase}/api/v1/public/dates`, {
		credentials: "omit",
		headers: { Accept: "application/json" },
	});
	const data = await response.json().catch(() => ({}));
	if (!response.ok) throw new Error(data.error ?? "No se pudo cargar la sala pública.");
	return data as PublicDate[];
}
