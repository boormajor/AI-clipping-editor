import React, {useEffect, useMemo, useState} from 'react';
import {AbsoluteFill, Easing, continueRender, delayRender, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import type {CaptionPreset, EditProps, Word} from './schema';
import {ensureFont} from './fonts';

type Entrance = 'fade' | 'pop' | 'slide' | 'bounce';
type WordFx = 'none' | 'color' | 'punch' | 'wipe' | 'box';
type CaptionSpec = {
  entrance: Entrance;
  word: WordFx;
  reveal: boolean;
  box: boolean;
  accentAll: boolean;
  stroke: boolean;
  uppercase: boolean;
};

/**
 * Caption presets. Word-level effects (reveal/punch/wipe/box) only apply when
 * measured word timings are available. With plain cues the whole cue is shown
 * with the group entrance animation; per-word timing is never fabricated.
 */
export const CAPTION_SPECS: Record<CaptionPreset, CaptionSpec> = {
  kinetic: {entrance: 'pop', word: 'punch', reveal: false, box: false, accentAll: false, stroke: true, uppercase: true},
  editorial: {entrance: 'fade', word: 'none', reveal: false, box: false, accentAll: false, stroke: false, uppercase: false},
  bold: {entrance: 'fade', word: 'none', reveal: false, box: false, accentAll: false, stroke: true, uppercase: true},
  minimal: {entrance: 'fade', word: 'none', reveal: false, box: false, accentAll: false, stroke: false, uppercase: false},
  karaoke: {entrance: 'fade', word: 'color', reveal: false, box: false, accentAll: false, stroke: true, uppercase: true},
  boxed: {entrance: 'pop', word: 'none', reveal: false, box: true, accentAll: false, stroke: false, uppercase: true},
  podcast: {entrance: 'fade', word: 'none', reveal: false, box: true, accentAll: false, stroke: false, uppercase: false},
  yellow: {entrance: 'fade', word: 'none', reveal: false, box: false, accentAll: true, stroke: true, uppercase: true},
  highlight: {entrance: 'fade', word: 'none', reveal: false, box: true, accentAll: true, stroke: false, uppercase: true},
  popline: {entrance: 'fade', word: 'punch', reveal: true, box: false, accentAll: false, stroke: true, uppercase: true},
  wordbox: {entrance: 'fade', word: 'box', reveal: false, box: false, accentAll: false, stroke: true, uppercase: true},
  wipe: {entrance: 'fade', word: 'wipe', reveal: false, box: false, accentAll: false, stroke: true, uppercase: true},
  slide: {entrance: 'slide', word: 'color', reveal: false, box: false, accentAll: false, stroke: true, uppercase: false},
  bounce: {entrance: 'bounce', word: 'punch', reveal: true, box: false, accentAll: false, stroke: true, uppercase: true},
  typewriter: {entrance: 'fade', word: 'none', reveal: true, box: false, accentAll: false, stroke: true, uppercase: false},
};

const POWER = new Set([
  'now', 'new', 'free', 'stop', 'never', 'always', 'secret', 'how', 'why', 'best', 'worst', 'biggest', 'mistake',
  'proven', 'instantly', 'fast', 'easy', 'money', 'growth', 'win', 'warning', 'huge', 'important', 'truth',
  'results', 'million', 'billion', 'everyone', 'nobody', 'everything', 'nothing', 'first', 'last', 'only',
]);

/** Whether a token deserves accent emphasis: a number, an acronym, or a power word. */
export function isEmphasis(raw: string): boolean {
  const w = raw.toLowerCase().replace(/[^a-z0-9]/g, '');
  if (!w) return false;
  if (/[0-9]/.test(w)) return true;
  const letters = raw.replace(/[^A-Za-z]/g, '');
  if (letters.length >= 2 && raw === raw.toUpperCase() && /[A-Z]/.test(raw)) return true;
  return POWER.has(w);
}

/** Group measured words by pauses, readable phrase length and a max word count. */
export function captionGroups(words: Word[], maxWords: number): Word[][] {
  const groups: Word[][] = [];
  let current: Word[] = [];
  for (const word of words) {
    if (current.length > 0) {
      const last = current[current.length - 1];
      const endsSentence = /[.!?]["']?$/.test(last.word);
      if (
        word.start - last.end > 0.32 ||
        word.end - current[0].start > 2.35 ||
        current.length >= maxWords ||
        (endsSentence && current.length >= Math.max(1, Math.floor(maxWords / 2)))
      ) {
        groups.push(current);
        current = [];
      }
    }
    current.push(word);
  }
  if (current.length) groups.push(current);
  return groups;
}

const clamp = (n: number, min: number, max: number) => Math.max(min, Math.min(max, n));

type WordSpanProps = {
  w: Word;
  spec: CaptionSpec;
  t: number;
  accent: string;
  base: string;
  emph: boolean;
  uppercase: boolean;
};

function WordSpan({w, spec, t, accent, base, emph, uppercase}: WordSpanProps) {
  const active = t >= w.start && t < w.end;
  const revealed = !spec.reveal || t >= w.start - 0.001;
  const popIn =
    spec.reveal && spec.word === 'punch'
      ? interpolate(t - w.start, [0, 0.08, 0.2], [0.6, 1.12, 1], {
          extrapolateLeft: 'clamp',
          extrapolateRight: 'clamp',
          easing: Easing.out(Easing.cubic),
        })
      : 1;
  const activePunch = active && spec.word === 'punch' ? 1.08 : 1;
  const style: React.CSSProperties & {WebkitBoxDecorationBreak?: string; WebkitTextFillColor?: string} = {
    display: 'inline-block',
    transform: `scale(${popIn * activePunch})`,
    transformOrigin: 'center bottom',
    opacity: revealed ? 1 : 0,
    color: base,
  };
  if (spec.word === 'box') {
    if (active) {
      style.background = accent;
      style.color = '#0b1020';
      style.borderRadius = '0.14em';
      style.padding = '0 0.16em';
      style.WebkitBoxDecorationBreak = 'clone';
      style.WebkitTextStroke = '0';
    }
  } else if (spec.word === 'wipe') {
    if (active) {
      const prog = clamp((t - w.start) / Math.max(0.08, w.end - w.start), 0, 1) * 100;
      style.backgroundImage = `linear-gradient(90deg, ${accent} ${prog}%, ${base} ${prog}%)`;
      style.WebkitBackgroundClip = 'text';
      style.backgroundClip = 'text';
      style.color = 'transparent';
      style.WebkitTextFillColor = 'transparent';
    } else {
      style.color = t >= w.end ? accent : base;
    }
  } else if (spec.word === 'color' || spec.word === 'punch') {
    style.color = active ? accent : base;
  }
  if (emph && !active && spec.word !== 'box' && spec.word !== 'wipe' && style.color === base && isEmphasis(w.word)) {
    style.color = accent;
  }
  const text = uppercase ? w.word.toUpperCase() : w.word;
  return (
    <>
      <span style={style}>{text}</span>{' '}
    </>
  );
}

/** Caption block anchored near the bottom safe area of the frame. */
export const CaptionLayer: React.FC<{p: EditProps}> = ({p}) => {
  const frame = useCurrentFrame();
  const {fps, width, height} = useVideoConfig();
  const font = useMemo(() => ensureFont(p.captionFont), [p.captionFont]);
  const [handle] = useState(() => delayRender(`Loading caption font ${p.captionFont}`));
  useEffect(() => {
    let cancelled = false;
    font.ready.finally(() => {
      if (!cancelled) continueRender(handle);
    });
    return () => {
      cancelled = true;
    };
  }, [font, handle]);

  const t = frame / fps - (p.captionOffset || 0);
  const aligned = p.words.length > 0;
  const groups = useMemo(() => (aligned ? captionGroups(p.words, clamp(p.captionWords, 1, 12)) : []), [aligned, p.words, p.captionWords]);

  if (!p.captions) return null;

  const idx = aligned ? groups.findIndex((g) => t >= g[0].start && t < g[g.length - 1].end) : -1;
  const group = idx >= 0 ? groups[idx] : undefined;
  const cueIdx = aligned ? -1 : p.cues.findIndex((c) => t >= c.start && t < c.end);
  const cue = cueIdx >= 0 ? p.cues[cueIdx] : undefined;
  if (!group && !cue) return null;

  const spec = CAPTION_SPECS[p.captionPreset] ?? CAPTION_SPECS.kinetic;
  const accent = p.accentColor;
  const baseColor = spec.accentAll ? accent : p.captionColor;
  const start = group ? group[0].start : cue!.start;
  const since = Math.max(0, t - start);

  // Skip the entrance when the previous group ended a moment ago: back-to-back
  // short groups would otherwise blink several times a second.
  const prevEnd = group && idx > 0 ? groups[idx - 1][groups[idx - 1].length - 1].end : cue && cueIdx > 0 ? p.cues[cueIdx - 1].end : -Infinity;
  const continuous = start - prevEnd < 0.18;
  const fade = continuous ? 1 : interpolate(since, [0, 0.1, 0.23], [0, 0.93, 1], {extrapolateRight: 'clamp'});
  let tx = 0;
  let scale = 1;
  if (!continuous) {
    if (spec.entrance === 'pop') {
      scale = interpolate(since, [0, 0.09, 0.22], [0.84, 1.06, 1], {extrapolateRight: 'clamp', easing: Easing.out(Easing.cubic)});
    } else if (spec.entrance === 'slide') {
      tx = interpolate(since, [0, 0.26], [54, 0], {extrapolateRight: 'clamp', easing: Easing.out(Easing.cubic)});
    } else if (spec.entrance === 'bounce') {
      scale = spring({fps, frame: Math.max(0, Math.round(since * fps)), config: {damping: 11, mass: 0.7, stiffness: 150}});
    }
  }

  // Scale typography with the shorter frame edge so 1:1 and 16:9 exports keep
  // the same visual weight as 9:16.
  const unit = Math.min(width, height) / 1080;
  const fontSize = clamp(p.captionSize, 24, 150) * unit;
  const stroke = spec.stroke ? Math.max(1.5, fontSize * 0.028) : 0;
  const text = cue?.text ?? group?.map((w) => w.word).join(' ') ?? '';
  if (!text.trim()) return null;
  const shownText = spec.uppercase ? text.toUpperCase() : text;

  return (
    <AbsoluteFill style={{justifyContent: 'flex-start', alignItems: 'center', pointerEvents: 'none'}}>
      <div
        style={{
          position: 'absolute',
          top: `${p.captionPosition * 100}%`,
          transform: `translateY(-100%) translateY(${tx}px) scale(${scale})`,
          maxWidth: '90%',
          padding: spec.box ? `${0.22 * fontSize}px ${0.34 * fontSize}px` : `${0.12 * fontSize}px ${0.1 * fontSize}px`,
          borderRadius: spec.box ? 0.3 * fontSize : 0.1 * fontSize,
          background: spec.box ? 'rgba(8,13,28,0.86)' : 'transparent',
          opacity: fade,
          textAlign: 'center',
          fontFamily: font.fontFamily,
          fontWeight: font.weight,
          fontSize,
          lineHeight: 1.12,
          letterSpacing: `${font.tracking}em`,
          color: baseColor,
          textShadow: spec.box ? '0 2px 8px rgba(0,0,0,0.5)' : `0 ${0.05 * fontSize}px ${0.06 * fontSize}px rgba(0,0,0,0.9), 0 0 ${0.16 * fontSize}px rgba(0,0,0,0.8)`,
          WebkitTextStroke: stroke ? `${stroke}px rgba(0,0,0,0.82)` : '0',
          paintOrder: 'stroke fill',
          overflowWrap: 'break-word',
        }}
      >
        {group
          ? group.map((w, i) => (
              <WordSpan key={`${w.start}-${i}`} w={w} spec={spec} t={t} accent={accent} base={baseColor} emph={p.emphasis} uppercase={spec.uppercase} />
            ))
          : p.emphasis
            ? shownText.split(/\s+/).map((tok, i) => (
                <React.Fragment key={i}>
                  <span style={{color: isEmphasis(tok) ? accent : baseColor}}>{tok}</span>{' '}
                </React.Fragment>
              ))
            : shownText}
      </div>
    </AbsoluteFill>
  );
};
