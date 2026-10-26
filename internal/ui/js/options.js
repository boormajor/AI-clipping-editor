// Labels for server-provided enumerations. The lists themselves come from
// /api/options so the UI never offers a value the renderer rejects.

export const PRESET_LABELS = {
  kinetic: 'Kinetic pop',
  popline: 'Word-by-word pop',
  typewriter: 'Typewriter',
  wordbox: 'Word box highlight',
  wipe: 'Karaoke wipe',
  karaoke: 'Karaoke fill',
  bounce: 'Bounce in',
  slide: 'Slide up',
  boxed: 'Boxed impact',
  podcast: 'Podcast clean',
  editorial: 'Editorial',
  bold: 'Modern bold',
  minimal: 'Minimal',
  yellow: 'Yellow pop',
  highlight: 'Yellow highlight',
};

/** Presets whose per-word effects need measured word timings. */
export const WORD_TIMED = new Set(['popline', 'typewriter', 'wordbox', 'wipe', 'karaoke', 'bounce', 'kinetic', 'slide']);

export const LAYOUT_LABELS = {
  focus: 'Focus — one speaker',
  'two-up': 'Both speakers — split',
  dynamic: 'AI shot plan',
};

export const LANGUAGE_LABELS = {
  auto: 'Automatic',
  en: 'English',
  ur: 'Urdu',
  hi: 'Hindi',
  es: 'Spanish',
  fr: 'French',
  de: 'German',
  it: 'Italian',
  pt: 'Portuguese',
  ar: 'Arabic',
  ja: 'Japanese',
  zh: 'Chinese',
  ko: 'Korean',
  ru: 'Russian',
  tr: 'Turkish',
  nl: 'Dutch',
  id: 'Indonesian',
};

export const SPEECH_LABELS = {
  groq: 'Groq Whisper (free tier available)',
  openai: 'OpenAI Whisper API (pay as you go)',
};

export const SPEECH_HINTS = {
  groq: 'Get a free key at console.groq.com/keys. A ChatGPT or Claude subscription is not a speech key.',
  openai: 'Create a key at platform.openai.com/api-keys. Billed separately from ChatGPT Plus.',
};

export const PROVIDER_LABELS = {
  heuristic: 'Local heuristic',
  chatgpt: 'ChatGPT plan',
  'claude-code': 'Claude (Claude Code)',
  openai: 'OpenAI API',
  anthropic: 'Anthropic API',
};

export const FEATURE_GROUPS = [
  ['Essential editing', [
    ['captions', 'Generate captions'],
    ['reframe', 'Vertical reframing with adjustable focus'],
    ['zoom', 'Subtle breathing zoom'],
  ]],
  ['Visuals', [
    ['broll', 'B-roll overlays'],
    ['graphics', 'Callout banners'],
    ['progressBar', 'Progress bar along the bottom edge'],
  ]],
];

export const PRESETS = {
  simple: {captions: true, reframe: true, zoom: false, broll: false, graphics: false, progressBar: false},
  advanced: {captions: true, reframe: true, zoom: true, broll: true, graphics: true, progressBar: true},
};
