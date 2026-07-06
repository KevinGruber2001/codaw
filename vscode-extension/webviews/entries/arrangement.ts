import { mount } from 'svelte';
import '../lib/theme.css';
import View from '../views/Arrangement.svelte';

mount(View, { target: document.getElementById('app')! });
