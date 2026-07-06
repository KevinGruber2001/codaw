import { mount } from 'svelte';
import '../lib/theme.css';
import View from '../views/BusEditor.svelte';

mount(View, { target: document.getElementById('app')! });
