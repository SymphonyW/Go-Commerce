// API服务：封装与后端API的通信
import axios from 'axios';

// API基础URL
const API_BASE_URL = 'http://localhost:8080/api';

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
  /**
   * 取消订单
   * @param {number} id - 订单ID
   * @returns {Promise} 取消结果
   */
  cancelOrder: async (id) => {
    const response = await api.put(`/orders/${id}/cancel`);
    return response.data;
  },
};

// 商户相关API
export const merchantAPI = {
	/**
	 * 创建商户
	 * @param {Object} merchantData - 商户数据
	 * @returns {Promise} 创建的商户
	 */
	createMerchant: async (merchantData) => {
		const response = await api.post('/merchants', merchantData);
		return response.data;
	},
	/**
	 * 获取商户详情
	 * @param {number} id - 商户ID
	 * @returns {Promise} 商户详情
	 */
	getMerchant: async (id) => {
		const response = await api.get(`/merchants/${id}`);
		return response.data;
	},
	/**
	 * 获取商户列表
	 * @returns {Promise} 商户列表
	 */
	listMerchants: async () => {
		const response = await api.get('/merchants');
		return response.data;
	},
	/**
	 * 商户添加产品
	 * @param {Object} productData - 产品数据
	 * @returns {Promise} 添加的产品
	 */
	addProduct: async (productData) => {
		const response = await api.post('/merchants/products', productData);
		return response.data;
	},
	/**
	 * 商户删除产品
	 * @param {Object} data - 包含商户ID和产品ID的数据
	 * @returns {Promise} 删除结果
	 */
	deleteProduct: async (data) => {
		const response = await api.delete('/merchants/products', { data });
		return response.data;
	},
};

// 购物车相关API
export const cartAPI = {
	/**
	 * 添加商品到购物车
	 * @param {Object} itemData - 商品数据
	 * @returns {Promise} 添加的购物车商品
	 */
	addItem: async (itemData) => {
		const response = await api.post('/cart/items', itemData);
		return response.data;
	},
	/**
	 * 获取购物车
	 * @returns {Promise} 购物车详情
	 */
	getCart: async () => {
		const response = await api.get('/cart');
		return response.data;
	},
	/**
	 * 更新购物车商品数量
	 * @param {Object} itemData - 商品数据
	 * @returns {Promise} 更新后的购物车商品
	 */
	updateItem: async (itemData) => {
		const response = await api.put('/cart/items', itemData);
		return response.data;
	},
	/**
	 * 删除购物车商品
	 * @param {Object} itemData - 商品数据
	 * @returns {Promise} 删除结果
	 */
	deleteItem: async (itemData) => {
		const response = await api.delete('/cart/items', { data: itemData });
		return response.data;
	},
	/**
	 * 清空购物车
	 * @returns {Promise} 清空结果
	 */
	clearCart: async () => {
		const response = await api.delete('/cart');
		return response.data;
	},
};

export default api;