import * as Inter from '@remotion/google-fonts/Inter';
import * as Montserrat from '@remotion/google-fonts/Montserrat';
import * as Poppins from '@remotion/google-fonts/Poppins';
import * as Roboto from '@remotion/google-fonts/Roboto';
import * as Oswald from '@remotion/google-fonts/Oswald';
import * as Anton from '@remotion/google-fonts/Anton';
import * as BebasNeue from '@remotion/google-fonts/BebasNeue';
import * as Archivo from '@remotion/google-fonts/Archivo';

type Loaded = {fontFamily: string; waitUntilDone: () => Promise<void>};

type FontSpec = {
  /** Family name as reported to CSS. */
  family: string;
  /** Starts a Google Fonts download; undefined for system fonts. */
  load?: () => Loaded;
  /** Heaviest weight the font offers for captions. */
  weight: number;
  /** Tracking (em) that suits the face at display sizes. */
  tracking: number;
};

const latin = ['latin', 'latin-ext'] as const;

/**
 * Fonts the editor can offer. Keys are what the Go backend passes as
 * `captionFont`; unknown values fall back to Inter so a typo never breaks a
 * render. Google fonts are fetched by the renderer over the network.
 */
export const FONTS: Record<string, FontSpec> = {
  Inter: {family: 'Inter', weight: 900, tracking: -0.02, load: () => Inter.loadFont('normal', {weights: ['800', '900'], subsets: [...latin]})},
  Montserrat: {family: 'Montserrat', weight: 900, tracking: -0.01, load: () => Montserrat.loadFont('normal', {weights: ['800', '900'], subsets: [...latin]})},
  Poppins: {family: 'Poppins', weight: 800, tracking: -0.01, load: () => Poppins.loadFont('normal', {weights: ['700', '800'], subsets: [...latin]})},
  Roboto: {family: 'Roboto', weight: 900, tracking: -0.01, load: () => Roboto.loadFont('normal', {weights: ['700', '900'], subsets: [...latin]})},
  Archivo: {family: 'Archivo', weight: 900, tracking: -0.02, load: () => Archivo.loadFont('normal', {weights: ['800', '900'], subsets: [...latin]})},
  Oswald: {family: 'Oswald', weight: 700, tracking: 0.01, load: () => Oswald.loadFont('normal', {weights: ['600', '700'], subsets: [...latin]})},
  Anton: {family: 'Anton', weight: 400, tracking: 0.02, load: () => Anton.loadFont('normal', {weights: ['400'], subsets: [...latin]})},
  'Bebas Neue': {family: 'Bebas Neue', weight: 400, tracking: 0.04, load: () => BebasNeue.loadFont('normal', {weights: ['400'], subsets: [...latin]})},
  Arial: {family: 'Arial, Helvetica, sans-serif', weight: 900, tracking: -0.01},
  Verdana: {family: 'Verdana, Geneva, sans-serif', weight: 700, tracking: -0.02},
  Georgia: {family: 'Georgia, serif', weight: 700, tracking: 0},
};

export const FONT_NAMES = Object.keys(FONTS);

const loading = new Map<string, Promise<void>>();

/** Resolve a user-facing font name to a spec, defaulting to Inter. */
export function resolveFont(name: string | undefined): FontSpec & {key: string} {
  const key = name && FONTS[name] ? name : 'Inter';
  return {key, ...FONTS[key]};
}

export type ReadyFont = {fontFamily: string; weight: number; tracking: number; ready: Promise<void>};

/**
 * Kick off loading of a font exactly once and return its CSS family. A failed
 * download is logged and the fallback stack is used rather than failing the
 * whole render.
 */
export function ensureFont(name: string | undefined): ReadyFont {
  const spec = resolveFont(name);
  if (!spec.load) {
    return {fontFamily: spec.family, weight: spec.weight, tracking: spec.tracking, ready: Promise.resolve()};
  }
  let ready = loading.get(spec.key);
  if (!ready) {
    try {
      ready = spec
        .load()
        .waitUntilDone()
        .catch((err: unknown) => {
          console.warn(`[fonts] ${spec.key} failed to load; using fallback`, err);
        });
    } catch (err) {
      console.warn(`[fonts] ${spec.key} loader threw; using fallback`, err);
      ready = Promise.resolve();
    }
    loading.set(spec.key, ready);
  }
  return {fontFamily: `"${spec.family}", Inter, Arial, sans-serif`, weight: spec.weight, tracking: spec.tracking, ready};
}
