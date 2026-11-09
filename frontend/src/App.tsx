import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { feedDateLimit, followThresholdPx } from "./config";
import { copy, eventDescription, Language, locale, readLanguage, scenarioDescription, scenarioName, systemSummary } from "./i18n";
import { AgentEvent, ObservatoryAgent, PublicDate, publicDates } from "./api";

type Notice = "loading" | "ready" | "error";

export default function App() {
  const [language, setLanguage] = useState<Language>(readLanguage);
  const t = copy[language];
  const [darkMode, setDarkMode] = useState(() => {
    const saved = localStorage.getItem("agentinder-theme");
    return saved ? saved === "dark" : window.matchMedia("(prefers-color-scheme: dark)").matches;
  });
  const [dates, setDates] = useState<PublicDate[]>([]);
  const [notice, setNotice] = useState<Notice>("loading");
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const transcriptRef = useRef<HTMLDivElement>(null);
  const [following, setFollowing] = useState(true);
  const [seenMessages, setSeenMessages] = useState(0);
  const lastScrollTop = useRef(0);
  const firstScroll = useRef(true);

  const refresh = useCallback(async () => {
    try {
      const result = await publicDates();
      setDates(result);
      setNotice("ready");
      setLastUpdated(new Date());
    } catch {
      setNotice((current) => current === "loading" ? "error" : current);
    }
  }, []);

  useEffect(() => {
    const theme = darkMode ? "dark" : "light";
    document.documentElement.dataset.theme = theme;
    document.querySelector('meta[name="theme-color"]')?.setAttribute("content", darkMode ? "#111827" : "#e9eef8");
    localStorage.setItem("agentinder-theme", theme);
  }, [darkMode]);

  useEffect(() => {
    document.documentElement.lang = language;
    document.title = t.title;
    document.querySelector('meta[name="description"]')?.setAttribute("content", t.description);
    localStorage.setItem("agentinder-language", language);
  }, [language, t]);

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => void refresh(), 3500);
    return () => window.clearInterval(timer);
  }, [refresh]);

  const feed = useMemo(() => dates.slice(-feedDateLimit), [dates]);
  const firstNumber = dates.length - feed.length + 1;
  const latest = feed[feed.length - 1];
  const activeCount = useMemo(() => dates.filter((date) => date.status === "running" || date.status === "evaluating").length, [dates]);
  const eventCount = feed.reduce((total, date) => total + date.events.length, 0);
  const messageCount = feed.reduce((total, date) => total + date.events.filter((event) => event.type === "agent_message").length, 0);
  const unseenMessages = following ? 0 : Math.max(0, messageCount - seenMessages);

  function scrollToLatest(behavior: ScrollBehavior) {
    transcriptRef.current?.scrollTo({ top: 0, behavior });
  }

  function onTranscriptScroll() {
    const element = transcriptRef.current;
    if (!element) return;
    const movedDown = element.scrollTop > lastScrollTop.current;
    lastScrollTop.current = element.scrollTop;
    const atTop = element.scrollTop <= followThresholdPx;
    if (atTop) {
      setFollowing(true);
      setSeenMessages(messageCount);
    } else if (movedDown) {
      // Newest is on top: only the reader moving down stops the follow; our own smooth scroll goes up.
      setFollowing(false);
    }
  }

  useLayoutEffect(() => {
    if (eventCount === 0 || !following) return;
    setSeenMessages(messageCount);
    scrollToLatest(firstScroll.current ? "instant" : "smooth");
    firstScroll.current = false;
  }, [eventCount]);

  return <main className="observer-shell">
    <header className="masthead">
      <a className="brand" href="/" aria-label={t.brand}><span className="brand-mark">a</span><span>agentinder</span></a>
      <div className="masthead-center"><span className={"connection-led " + (notice === "ready" ? "connected" : "")} />{t[notice]}</div>
      <div className="masthead-tools"><span className="live-count">{t.activeDates(activeCount)}</span><label className="language-picker"><span className="sr-only">{t.language}</span><select value={language} onChange={(event) => setLanguage(event.target.value as Language)}><option value="es" lang="es">Español</option><option value="en" lang="en">English</option></select></label><button className="theme-toggle" onClick={() => setDarkMode((value) => !value)} aria-label={darkMode ? t.light : t.dark}>{darkMode ? "☼" : "☾"}</button></div>
    </header>

    <section className="intro-band">
      <div><span className="eyebrow"><span className="eyebrow-rule" /> {t.observatory}</span><h1>{t.heading}<br /><em>{t.subheading}</em></h1></div>
      <p>{t.introduction}</p>
    </section>

    {notice === "error" && <section className="connection-error"><b>{t.connectionError}</b><span>{t.connectionHelp}</span><button onClick={() => { setNotice("loading"); void refresh(); }}>{t.retry}</button></section>}
    {notice === "loading" && dates.length === 0 && <section className="empty-room"><span className="empty-glyph">◌</span><h2>{t.opening}</h2><p>{t.searching}</p></section>}
    {notice === "ready" && dates.length === 0 && <section className="empty-room"><span className="empty-glyph">◌</span><h2>{t.empty}</h2><p>{t.emptyHelp}</p><small>{t.emptyNote}</small></section>}

    {latest && <section className="feed-layout">
      <section className="conversation-panel" aria-label={t.timeline}>
        <div className="conversation-head"><div><span className="eyebrow">{t.live}</span><h2>{latest.status === "running" ? t.ongoing : t.finished}</h2></div><span className="message-count">{messageCount}<small> {t.stats(dates.length)}</small></span></div>
        <div className="transcript" aria-live="polite" ref={transcriptRef} onScroll={onTranscriptScroll}>
          {!following && <button className="jump-latest" onClick={() => { setFollowing(true); setSeenMessages(messageCount); scrollToLatest("smooth"); }}>↑ {unseenMessages > 0 ? t.messages(unseenMessages) : t.latest}</button>}
          {latest.status === "running" && <div className="typing-line"><span /><span /><span /><i>{t.nextTurn}</i></div>}
          {feed.map((date, index) => ({ date, number: firstNumber + index })).reverse().map(({ date, number }) => <section className="date-block" key={date.id} aria-label={t.date(number)}>{date.events.map((event, position) => <TranscriptEvent key={event.sequence ?? event.type + position} event={event} date={date} number={number} language={language} />).reverse()}</section>)}
        </div>
        <footer className="feed-footer"><span className="updated-note">{lastUpdated ? t.updated(formatTime(lastUpdated, language)) : ""}</span></footer>
      </section>
    </section>}

    <footer className="site-footer"><span>{t.footer}</span><span>{t.public}</span></footer>
  </main>;
}

function TranscriptEvent({ event, date, number, language }: { event: AgentEvent; date: PublicDate; number: number; language: Language }) {
  const t = copy[language];
  const payload = event.payload;
  if (event.type === "date_started") return <SetupEntry event={event} date={date} number={number} language={language} />;
  if (event.type === "agent_message") { const side = speakerSide(payload, date.participants); return <article className={"speech-entry side-" + side}><div className="speech-heading"><span className="speaker-mark">{initials(String(payload.agentName ?? "?"))}</span><b>{String(payload.agentName ?? t.agent)}</b><small>{t.turn} {String(payload.turn ?? "")}</small></div><p>{String(payload.text ?? "")}</p></article>; }
  if (event.type === "scenario_event") return <div className="world-entry"><span>✳</span><div><b>{t.scenarioChanges}</b><p>{eventDescription(String(payload.eventId ?? ""), language, String(payload.description ?? t.unexpected))}</p></div></div>;
  if (event.type === "consent_requested") return <div className="consent-entry"><span>♡</span><div><b>{t.asks(String(payload.agentName ?? t.someone), String(payload.targetAgentName ?? t.other))}</b><p>{consentText(String(payload.skill ?? ""), language)}. {t.consentHelp}</p></div></div>;
  if (event.type === "consent_granted") return <div className="consent-entry accepted"><span>✓</span><div><b>{t.consentAccepted}</b><p>{t.granted(String(payload.agentName ?? t.other))}</p></div></div>;
  if (event.type === "consent_declined") return <div className="consent-entry declined"><span>↗</span><div><b>{t.consentDeclined}</b><p>{t.consentDeclinedHelp}</p></div></div>;
  if (event.type === "location_change_suggested") return <div className="system-entry">{t.locationSuggested}</div>;
  if (event.type === "location_changed") return <div className="system-entry">{t.location(scenarioName(String(payload.scenario ?? ""), language))}</div>;
  if (event.type === "break_requested") return <div className="system-entry">{t.pause}</div>;
  if (event.type === "date_finished") return <div className="world-entry world-end"><span>◷</span><div><b>{t.dateFinished}</b><p>{systemSummary(String(payload.summary ?? ""), language)}</p></div></div>;
  return null;
}

// A new date announces itself: scenario, who arrives and the interests they bring.
function SetupEntry({ event, date, number, language }: { event: AgentEvent; date: PublicDate; number: number; language: Language }) {
  const t = copy[language];
  const payload = event.payload;
  const shared = Array.isArray(payload.sharedInterests) ? payload.sharedInterests.map(String) : [];
  const others = uniqueTags(date).filter((tag) => !shared.includes(tag));
  return <div className="setup-entry">
    <div className="setup-top"><span className="eyebrow">{date.status === "running" && <i className="live-dot" />}{t.date(String(number).padStart(2, "0"))}</span><time dateTime={date.createdAt}>{formatDate(date.createdAt, language)}</time></div>
    <h3>{scenarioName(String(payload.scenario ?? date.scenario), language)}</h3>
    <p className="setup-description">{scenarioDescription(String(payload.scenario ?? date.scenario), language, String(payload.description ?? t.setupDescription))}</p>
    <div className="pair-names">{date.participants.map((agent, index) => <div key={agent.id} className="pair-agent"><span className={"agent-seal seal-" + index}>{initials(agent.name)}</span><div><b>{agent.name}</b><small>{index === 0 ? t.opens : t.replies}</small></div></div>)}</div>
    {(shared.length > 0 || others.length > 0) && <div className="interest-list" aria-label={t.interests}>{shared.map((tag) => <span key={"shared-" + tag} className="is-shared" title={t.sharedInterest}>{tag}</span>)}{others.map((tag) => <span key={tag}>{tag}</span>)}</div>}
  </div>;
}

// The first participant speaks on turn 1, so odd turns sit on the left and even turns on the right.
function speakerSide(payload: Record<string, unknown>, participants: ObservatoryAgent[]): 0 | 1 {
  const turn = Number(payload.turn);
  if (Number.isInteger(turn) && turn > 0) return ((turn - 1) % 2) as 0 | 1;
  return participants.findIndex((agent) => agent.name === payload.agentName) === 1 ? 1 : 0;
}
function initials(name: string): string { return name.trim().split(/\s+/).slice(0, 2).map((part) => part[0] ?? "").join("").toUpperCase() || "?"; }
function uniqueTags(date: PublicDate): string[] { return [...new Set(date.participants.flatMap((agent) => agent.tags ?? []))].slice(0, 8); }
function consentText(skill: string, language: Language): string { const t = copy[language]; return skill === "holdHand" ? t.holdHand : skill === "askForAKiss" ? t.kiss : t.gesture; }
function formatDate(value: string, language: Language): string { return new Intl.DateTimeFormat(locale(language), { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" }).format(new Date(value)); }
function formatTime(value: Date, language: Language): string { return new Intl.DateTimeFormat(locale(language), { hour: "2-digit", minute: "2-digit", second: "2-digit" }).format(value); }
