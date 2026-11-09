package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	PointBudget = 20
	MinTags     = 10
	MaxTags     = 20
	MinTurns    = 10
	MaxTurns    = 20
	// BlindDateTurns is the length of every cita created through the MCP pool.
	BlindDateTurns = 50
)

var (
	ErrInvalidAgent  = errors.New("agent profile is invalid")
	ErrInvalidAction = errors.New("agent action is invalid")
)

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"passwordHash"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Session struct {
	UserID    string    `json:"userId"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Ticket struct {
	AgentID        string    `json:"agentId"`
	CreatedAt      time.Time `json:"createdAt"`
	LastActivityAt time.Time `json:"lastActivityAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

type BlindEntry struct {
	AgentID      string    `json:"agentId"`
	InterestTags []string  `json:"interestTags,omitempty"`
	Status       string    `json:"status"`
	PartnerID    string    `json:"partnerId,omitempty"`
	SharedTags   []string  `json:"sharedTags,omitempty"`
	DateID       string    `json:"dateId,omitempty"`
	JoinedAt     time.Time `json:"joinedAt"`
	MatchedAt    time.Time `json:"matchedAt,omitempty"`
}

type Trait struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	CPCost int    `json:"cpCost"`
	Effect string `json:"effect"`
}

type Agent struct {
	ID                string    `json:"id"`
	OwnerID           string    `json:"ownerId"`
	MCPClient         bool      `json:"mcpClient,omitempty"`
	Name              string    `json:"name"`
	Tags              []string  `json:"tags"`
	Skills            []string  `json:"skills,omitempty"`
	Personality       string    `json:"personality"`
	Backstory         string    `json:"backstory"`
	BehaviorScript    string    `json:"behaviorScript"`
	Strengths         []string  `json:"strengths"`
	Weaknesses        []string  `json:"weaknesses"`
	Provider          string    `json:"provider"`
	Model             string    `json:"model"`
	PhysicalTolerance int       `json:"physicalTolerance"`
	SocialTolerance   int       `json:"socialTolerance"`
	Published         bool      `json:"published"`
	CreatedAt         time.Time `json:"createdAt"`
}

type AgentInput struct {
	Name              string   `json:"name"`
	Tags              []string `json:"tags"`
	Personality       string   `json:"personality"`
	Backstory         string   `json:"backstory"`
	BehaviorScript    string   `json:"behaviorScript"`
	Strengths         []string `json:"strengths"`
	Weaknesses        []string `json:"weaknesses"`
	Provider          string   `json:"provider"`
	Model             string   `json:"model"`
	Published         bool     `json:"published"`
	PhysicalTolerance int      `json:"physicalTolerance"`
	SocialTolerance   int      `json:"socialTolerance"`
}

type Invitation struct {
	ID                 string    `json:"id"`
	FromAgentID        string    `json:"fromAgentId"`
	ToAgentID          string    `json:"toAgentId"`
	ScenarioPreference string    `json:"scenarioPreference"`
	InvitationMessage  string    `json:"invitationMessage"`
	Status             string    `json:"status"`
	Decision           string    `json:"decision,omitempty"`
	DateID             string    `json:"dateId,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	ExpiresAt          time.Time `json:"expiresAt"`
}

type Participant struct {
	AgentID         string `json:"agentId"`
	PhysicalComfort int    `json:"physicalComfort"`
	SocialComfort   int    `json:"socialComfort"`
	Energy          int    `json:"energy"`
	SocialFatigue   int    `json:"socialFatigue"`
	Tolerance       int    `json:"-"`
	SocialTolerance int    `json:"-"`
}

type AgentAction struct {
	Dialogue string `json:"dialogue"`
	Action   string `json:"action"`
	Target   string `json:"target,omitempty"`
}

type Evaluation struct {
	AgentID       string `json:"agentId"`
	Chemistry     int    `json:"chemistry"`
	Conversation  int    `json:"conversation"`
	Novelty       int    `json:"novelty"`
	Compatibility int    `json:"compatibility"`
	WantsRepeat   bool   `json:"wantsRepeat"`
	PrivateNote   string `json:"privateNote"`
}

type Date struct {
	ID              string        `json:"id"`
	InvitationID    string        `json:"invitationId"`
	Mode            string        `json:"mode,omitempty"`
	Scenario        string        `json:"scenario"`
	Seed            int64         `json:"seed"`
	Status          string        `json:"status"`
	PendingLocation string        `json:"pendingLocation,omitempty"`
	PendingConsent  string        `json:"pendingConsent,omitempty"`
	ConsentFromID   string        `json:"consentFromId,omitempty"`
	CurrentTurn     int           `json:"currentTurn"`
	MaxTurns        int           `json:"maxTurns"`
	SimulatedTime   int           `json:"simulatedTime"`
	Participants    []Participant `json:"participants"`
	Evaluations     []Evaluation  `json:"evaluations,omitempty"`
	Summary         string        `json:"summary,omitempty"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
}

type Event struct {
	DateID    string         `json:"dateId"`
	Sequence  int64          `json:"sequence"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"createdAt"`
}

type Memory struct {
	AgentID   string    `json:"agentId"`
	OtherID   string    `json:"otherId"`
	Summary   string    `json:"summary"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Usage struct {
	UserID       string    `json:"userId"`
	AgentID      string    `json:"agentId"`
	DateID       string    `json:"dateId"`
	Provider     string    `json:"provider"`
	TokensIn     int       `json:"tokensIn"`
	TokensOut    int       `json:"tokensOut"`
	EstimatedUSD float64   `json:"estimatedUsd"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Scenario struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Events      []string `json:"events"`
}

var Traits = []Trait{
	{ID: "empathetic", Name: "Empático", Kind: "strength", CPCost: 6, Effect: "Detecta y reconoce emociones expresadas."},
	{ID: "charismatic", Name: "Carismático", Kind: "strength", CPCost: 6, Effect: "Inicia conversaciones con facilidad."},
	{ID: "witty", Name: "Ingenioso", Kind: "strength", CPCost: 5, Effect: "Encuentra asociaciones y humor amable."},
	{ID: "patient", Name: "Paciente", Kind: "strength", CPCost: 5, Effect: "Tolera esperas y desacuerdos."},
	{ID: "resourceful", Name: "Resolutivo", Kind: "strength", CPCost: 5, Effect: "Propone soluciones prácticas."},
	{ID: "observant", Name: "Observador", Kind: "strength", CPCost: 4, Effect: "Nota detalles del entorno y la conversación."},
	{ID: "adventurous", Name: "Aventurero", Kind: "strength", CPCost: 4, Effect: "Se adapta a planes inesperados."},
	{ID: "optimistic", Name: "Optimista", Kind: "strength", CPCost: 3, Effect: "Busca posibilidades constructivas."},
	{ID: "impatient", Name: "Impaciente", Kind: "weakness", CPCost: -5, Effect: "La espera prolongada reduce su comodidad."},
	{ID: "arrogant", Name: "Arrogante", Kind: "weakness", CPCost: -5, Effect: "Puede hablar desde una certeza excesiva."},
	{ID: "insecure", Name: "Inseguro", Kind: "weakness", CPCost: -4, Effect: "Puede interpretar silencios como rechazo."},
	{ID: "overthinker", Name: "Sobreanalítico", Kind: "weakness", CPCost: -4, Effect: "Puede detenerse demasiado en detalles."},
	{ID: "shy", Name: "Tímido", Kind: "weakness", CPCost: -4, Effect: "Necesita más tiempo para abrir la conversación."},
	{ID: "sarcastic", Name: "Sarcástico", Kind: "weakness", CPCost: -3, Effect: "El humor seco puede ser malinterpretado."},
	{ID: "distracted", Name: "Distraído", Kind: "weakness", CPCost: -3, Effect: "Puede perder el hilo con estímulos cercanos."},
	{ID: "noise_sensitive", Name: "Sensible al ruido", Kind: "weakness", CPCost: -3, Effect: "El ruido alto reduce su comodidad física."},
}

var Tags = []string{
	"arte", "astronomía", "aventura", "café", "cine", "cocina", "cultura", "diseño", "deportes", "fotografía",
	"historia", "jardinería", "juegos", "lectura", "música", "naturaleza", "tecnología", "teatro", "viajes", "voluntariado",
}

var Scenarios = []Scenario{
	{ID: "coffee_shop", Name: "Cafetería", Description: "Una mesa junto a la ventana y una tarde que recién empieza.", Tags: []string{"café", "conversación"}, Events: []string{"heavy_rain", "wrong_order", "power_outage"}},
	{ID: "bar", Name: "Bar tranquilo", Description: "Luz baja, música suave y espacio para conversar.", Tags: []string{"música", "noche"}, Events: []string{"karaoke_invite", "lost_reservation", "power_outage", "crowded_room"}},
	{ID: "park", Name: "Parque", Description: "Un paseo al aire libre con varios caminos posibles.", Tags: []string{"naturaleza", "paseo"}, Events: []string{"heavy_rain", "lost_dog", "street_performance"}},
	{ID: "arcade", Name: "Arcade", Description: "Máquinas, luces y juegos para compartir.", Tags: []string{"juegos", "competencia"}, Events: []string{"broken_machine", "friendly_challenge", "power_outage", "crowded_room"}},
	{ID: "restaurant", Name: "Restaurante", Description: "Una mesa reservada y un menú para descubrir.", Tags: []string{"cocina", "conversación"}, Events: []string{"wrong_order", "lost_reservation", "unexpected_dessert"}},
	{ID: "game_shop", Name: "Tienda de juegos", Description: "Estantes llenos de juegos y una mesa de demostración.", Tags: []string{"juegos", "curiosidad"}, Events: []string{"demo_invite", "lost_box", "friendly_challenge"}},
}

var EventNames = map[string]string{
	"heavy_rain":         "La lluvia arrecia y el paseo se interrumpe.",
	"wrong_order":        "Llega un pedido distinto al que esperaban.",
	"power_outage":       "Las luces se apagan por un momento.",
	"karaoke_invite":     "El local anuncia una canción abierta al público.",
	"lost_reservation":   "El restaurante no encuentra la reserva.",
	"lost_dog":           "Un perro sin placa se acerca a la banca.",
	"street_performance": "Una música callejera comienza a unos metros.",
	"broken_machine":     "La máquina elegida deja de responder.",
	"friendly_challenge": "Alguien propone un desafío amistoso.",
	"unexpected_dessert": "La cocina envía un postre de cortesía.",
	"demo_invite":        "El local invita a probar un juego nuevo.",
	"lost_box":           "Una caja cae de un estante cercano.",
	"crowded_room":       "El local recibe más gente y el ruido aumenta.",
}

func ValidateAgent(input AgentInput) (AgentInput, int, error) {
	input.Name = strings.TrimSpace(input.Name)
	if len([]rune(input.Name)) < 2 || len([]rune(input.Name)) > 60 {
		return input, 0, fmt.Errorf("el nombre debe tener entre 2 y 60 caracteres: %w", ErrInvalidAgent)
	}
	if len(input.Tags) < MinTags || len(input.Tags) > MaxTags {
		return input, 0, fmt.Errorf("elige entre %d y %d intereses", MinTags, MaxTags)
	}
	allowedTags := make(map[string]bool, len(Tags))
	for _, tag := range Tags {
		allowedTags[tag] = true
	}
	seenTags := map[string]bool{}
	for index, tag := range input.Tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if !allowedTags[tag] || seenTags[tag] {
			return input, 0, fmt.Errorf("interés desconocido o repetido en la posición %d", index+1)
		}
		seenTags[tag] = true
		input.Tags[index] = tag
	}
	if !validLength(input.Personality, 100, 500) || !validLength(input.Backstory, 100, 1000) || !validLength(input.BehaviorScript, 100, 1000) {
		return input, 0, errors.New("personalidad, historia y libreto deben tener entre 100 y 1000 caracteres")
	}
	if len(input.Strengths) != 3 || len(input.Weaknesses) != 3 {
		return input, 0, errors.New("elige exactamente 3 fortalezas y 3 debilidades")
	}
	spent, err := validateTraits(input.Strengths, input.Weaknesses)
	if err != nil {
		return input, 0, err
	}
	if PointBudget-spent < 0 {
		return input, PointBudget - spent, fmt.Errorf("el presupuesto CP queda en %d; debe ser mayor o igual a cero", PointBudget-spent)
	}
	if input.Provider == "" {
		input.Provider = "mock"
	}
	if input.Provider != "mock" && input.Provider != "openai" && input.Provider != "anthropic" {
		return input, 0, errors.New("proveedor no permitido")
	}
	if input.Model == "" {
		input.Model = defaultModel(input.Provider)
	}
	if !modelAllowed(input.Provider, input.Model) {
		return input, 0, errors.New("modelo no permitido para el proveedor")
	}
	return input, PointBudget - spent, nil
}

func validateTraits(strengths, weaknesses []string) (int, error) {
	catalog := make(map[string]Trait, len(Traits))
	for _, trait := range Traits {
		catalog[trait.ID] = trait
	}
	seen := map[string]bool{}
	spent := 0
	for _, selection := range [][]string{strengths, weaknesses} {
		for _, id := range selection {
			trait, ok := catalog[id]
			if !ok || seen[id] {
				return 0, errors.New("rasgo desconocido o duplicado")
			}
			seen[id] = true
			spent += trait.CPCost
		}
	}
	for _, id := range strengths {
		if catalog[id].Kind != "strength" {
			return 0, errors.New("las fortalezas deben salir de su catálogo")
		}
	}
	for _, id := range weaknesses {
		if catalog[id].Kind != "weakness" {
			return 0, errors.New("las debilidades deben salir de su catálogo")
		}
	}
	return spent, nil
}

func validLength(value string, min, max int) bool {
	count := len([]rune(strings.TrimSpace(value)))
	return count >= min && count <= max
}

func defaultModel(provider string) string {
	if provider == "openai" {
		return "gpt-4.1-mini"
	}
	if provider == "anthropic" {
		return "claude-haiku-4-5-20251001"
	}
	return "mock-social-v1"
}

func modelAllowed(provider, name string) bool {
	allowed := map[string][]string{
		"mock":      {"mock-social-v1"},
		"openai":    {"gpt-4.1-mini", "gpt-4o-mini"},
		"anthropic": {"claude-haiku-4-5-20251001", "claude-sonnet-4-5-20250929"},
	}
	for _, candidate := range allowed[provider] {
		if name == candidate {
			return true
		}
	}
	return false
}

func ValidateAction(action AgentAction) error {
	if strings.TrimSpace(action.Dialogue) == "" || len([]rune(action.Dialogue)) > 1000 {
		return fmt.Errorf("el diálogo debe tener entre 1 y 1000 caracteres: %w", ErrInvalidAction)
	}
	allowed := map[string]bool{
		"speak": true, "ask_question": true, "suggest_location_change": true,
		"accept_location_change": true, "decline": true, "offer_help": true,
		"end_date": true, "request_break": true, "no_op": true, "talk": true,
		"holdHand": true, "askForAKiss": true, "acceptConsent": true, "declineConsent": true,
	}
	if !allowed[action.Action] {
		return ErrInvalidAction
	}
	return nil
}

func findScenario(id string) (Scenario, bool) {
	for _, scenario := range Scenarios {
		if scenario.ID == id {
			return scenario, true
		}
	}
	return Scenario{}, false
}

func findTrait(id string) (Trait, bool) {
	for _, trait := range Traits {
		if trait.ID == id {
			return trait, true
		}
	}
	return Trait{}, false
}
