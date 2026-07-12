import { render } from 'preact';
import { Card } from './card';
import './card.css';

const root = document.getElementById('app');

if (!root) {
  throw new Error('CodeCard root is missing');
}

render(<Card />, root);
