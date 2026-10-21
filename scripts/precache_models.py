"""Precache WhisperX models for first-run setup."""
import torch
import whisperx

device = 'cuda' if torch.cuda.is_available() else 'cpu'
whisperx.load_model('small', device=device, compute_type='float16' if device == 'cuda' else 'int8', language='en')
whisperx.load_align_model(language_code='en', device=device)
print('Speech models cached')
