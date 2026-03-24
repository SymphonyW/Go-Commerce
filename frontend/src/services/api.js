// API服务：封装与后端API的通信
import axios from 'axios';

// API基础URL
const API_BASE_URL = 'http://localhost:8081/api';

// 创建axios实例
const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// 请求拦截器：添加认证令牌
api.interceptors.request.use(
  (config) => {
    // 从本地存储获取令牌
    const token = localStorage.getItem('token');
    if (token) {
      // 添加Bearer令牌到请求头
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// 认证相关API
export const authAPI = {
  /**
   * 用户注册
   * @param {Object} userData - 用户注册数据
   * @returns {Promise} 注册结果
   */
  register: async (userData) => {
    const response = await api.post('/register', userData);
    return response.data;
  },
  /**
   * 用户登录
   * @param {Object} credentials - 用户登录凭据
   * @returns {Promise} 登录结果，包含令牌
   */
  login: async (credentials) => {
    const response = await api.post('/login', credentials);
    return response.data;
  },
};

// 产品相关API
export const productAPI = {
  /**
   * 获取产品列表
   * @returns {Promise} 产品列表
   */
  listProducts: async () => {
    const response = await api.get('/products');
    return response.data;
  },
  /**
   * 获取产品详情
   * @param {number} id - 产品ID
   * @returns {Promise} 产品详情
   */
  getProduct: async (id) => {
    const response = await api.get(`/products/${id}`);
    return response.data;
  },
};

// 订单相关API
export const orderAPI = {
  /**
   * 创建订单
   * @param {Object} orderData - 订单数据
   * @returns {Promise} 创建的订单
   */
  createOrder: async (orderData) => {
    const response = await api.post('/orders', orderData);
    return response.data;
  },
  /**
   * 获取订单详情
   * @param {number} id - 订单ID
   * @returns {Promise} 订单详情
   */
  getOrder: async (id) => {
    const response = await api.get(`/orders/${id}`);
    return response.data;
  },
  /**
   * 获取订单列表
   * @returns {Promise} 订单列表
   */
  listOrders: async () => {
    const response = await api.get('/orders');
    return response.data;
  },
};

export default api;